package render

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// RenderContext holds the context for template rendering
type RenderContext struct {
	Variables       map[string]any
	PackName        string
	PackDescription string
	PackVersion     string
}

// Engine renders Nomad pack templates
type Engine struct {
	ctx *RenderContext
}

// NewEngine creates a new rendering engine
func NewEngine() *Engine {
	return &Engine{
		ctx: &RenderContext{
			Variables: make(map[string]any),
		},
	}
}

// SetPackMetadata sets pack metadata for the "meta" template function
func (e *Engine) SetPackMetadata(name, description, version string) {
	e.ctx.PackName = name
	e.ctx.PackDescription = description
	e.ctx.PackVersion = version
}

// SetVariables sets variable values for the "var" template function
func (e *Engine) SetVariables(vars map[string]any) {
	for k, v := range vars {
		e.ctx.Variables[k] = v
	}
}

// SetVariable sets a single variable value
func (e *Engine) SetVariable(name string, value any) {
	e.ctx.Variables[name] = value
}

// RenderPack renders all templates in a pack directory
func (e *Engine) RenderPack(packDir string) (string, error) {
	templatesDir := filepath.Join(packDir, "templates")

	// Check if templates directory exists
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		return "", fmt.Errorf("templates directory not found: %s", templatesDir)
	}

	// Read all template files
	var helperContent strings.Builder
	var mainTemplates []string

	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return "", fmt.Errorf("failed to read templates directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".tpl") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(templatesDir, name))
		if err != nil {
			return "", fmt.Errorf("failed to read template %s: %w", name, err)
		}

		// Helper templates start with underscore
		if strings.HasPrefix(name, "_") {
			helperContent.Write(content)
			helperContent.WriteString("\n")
		} else {
			mainTemplates = append(mainTemplates, string(content))
		}
	}

	// Combine helper templates with main templates
	var result strings.Builder
	for i, mainTpl := range mainTemplates {
		if i > 0 {
			result.WriteString("\n---\n")
		}

		rendered, err := e.RenderTemplate(helperContent.String() + mainTpl)
		if err != nil {
			return "", err
		}
		result.WriteString(rendered)
	}

	return result.String(), nil
}

// RenderTemplate renders a single template string
func (e *Engine) RenderTemplate(content string) (string, error) {
	// Create template with custom delimiters [[ and ]]
	tmpl := template.New("pack").Delims("[[", "]]").Funcs(TemplateFuncs(e.ctx))

	// Parse the template
	parsed, err := tmpl.Parse(content)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	// Execute the template
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, e.ctx); err != nil {
		return "", fmt.Errorf("template execution error: %w", err)
	}

	return buf.String(), nil
}

// RenderFile renders a single template file
func (e *Engine) RenderFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return e.RenderTemplate(string(content))
}
