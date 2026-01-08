package cmd

import (
	"fmt"
	"os"

	"rmbl/internal/pack"
	"rmbl/internal/render"

	"github.com/spf13/cobra"
)

var (
	renderVars    []string
	renderVarFile string
	renderOutput  string
)

var packRenderCmd = &cobra.Command{
	Use:   "render <pack-path>",
	Short: "Render pack templates without submitting to Nomad",
	Long: `Render a local pack's templates and output the resulting job specification.

This command is useful for testing pack templates before running them.

Examples:
  ramble pack render ./my-pack
  ramble pack render ./my-pack --var count=3 --var message="Hello"
  ramble pack render ./my-pack --output job.nomad.hcl`,
	Args: cobra.ExactArgs(1),
	RunE: runPackRender,
}

func init() {
	packCmd.AddCommand(packRenderCmd)

	packRenderCmd.Flags().StringArrayVarP(&renderVars, "var", "v", nil, "Variable override (key=value)")
	packRenderCmd.Flags().StringVar(&renderVarFile, "var-file", "", "Variable file (JSON)")
	packRenderCmd.Flags().StringVarP(&renderOutput, "output", "o", "", "Output file (default: stdout)")
}

func runPackRender(cmd *cobra.Command, args []string) error {
	packPath := args[0]

	// Check if pack path exists
	if _, err := os.Stat(packPath); os.IsNotExist(err) {
		return fmt.Errorf("pack path does not exist: %s", packPath)
	}

	// Load pack metadata
	metadata, err := loadPackMetadata(packPath)
	if err != nil {
		// Non-fatal - just use empty metadata
		metadata = &packMetadata{}
	}

	// Load variables defaults from variables.hcl
	variables := make(map[string]any)
	varsPath := packPath + "/variables.hcl"
	if _, err := os.Stat(varsPath); err == nil {
		content, err := os.ReadFile(varsPath)
		if err == nil {
			if defs, err := parseVariablesSimple(string(content)); err == nil {
				for k, v := range defs {
					variables[k] = v
				}
			}
		}
	}

	// Load var file if specified
	if renderVarFile != "" {
		fileVars, err := pack.ParseVarFile(renderVarFile)
		if err != nil {
			return fmt.Errorf("failed to load var file: %w", err)
		}
		for k, v := range fileVars {
			variables[k] = v
		}
	}

	// Parse CLI var flags
	for _, v := range renderVars {
		key, val, err := pack.ParseVarFlag(v)
		if err != nil {
			return err
		}
		variables[key] = val
	}

	// Create render engine
	engine := render.NewEngine()
	engine.SetPackMetadata(metadata.Name, metadata.Description, metadata.Version)
	engine.SetVariables(variables)

	// Render the pack
	result, err := engine.RenderPack(packPath)
	if err != nil {
		return fmt.Errorf("failed to render pack: %w", err)
	}

	// Output result
	if renderOutput != "" {
		if err := os.WriteFile(renderOutput, []byte(result), 0644); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
		fmt.Printf("Rendered to %s\n", renderOutput)
	} else {
		fmt.Print(result)
	}

	return nil
}

// packMetadata holds pack metadata from metadata.hcl
type packMetadata struct {
	Name        string
	Description string
	Version     string
}

// loadPackMetadata loads metadata from a pack's metadata.hcl
func loadPackMetadata(packPath string) (*packMetadata, error) {
	metaPath := packPath + "/metadata.hcl"
	content, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	// Simple parsing - look for pack block
	meta := &packMetadata{}
	lines := string(content)

	// Extract name
	if idx := findValue(lines, "name"); idx != "" {
		meta.Name = idx
	}
	if idx := findValue(lines, "description"); idx != "" {
		meta.Description = idx
	}
	if idx := findValue(lines, "version"); idx != "" {
		meta.Version = idx
	}

	return meta, nil
}

// findValue extracts a value from HCL-like content
func findValue(content, key string) string {
	// Simple regex-free parser for key = "value" patterns
	lines := splitLines(content)
	for _, line := range lines {
		line = trimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		if idx := indexString(line, key); idx >= 0 {
			// Found key, look for = and value
			rest := line[idx+len(key):]
			rest = trimSpace(rest)
			if len(rest) > 0 && rest[0] == '=' {
				rest = trimSpace(rest[1:])
				if len(rest) > 0 && rest[0] == '"' {
					// Find closing quote
					end := 1
					for end < len(rest) && rest[end] != '"' {
						end++
					}
					if end < len(rest) {
						return rest[1:end]
					}
				}
			}
		}
	}
	return ""
}

// parseVariablesSimple does simple extraction of variable defaults
func parseVariablesSimple(content string) (map[string]any, error) {
	vars := make(map[string]any)

	// Simple state machine to find variable blocks and their defaults
	lines := splitLines(content)
	var currentVar string
	var inBlock bool
	var braceDepth int

	for _, line := range lines {
		line = trimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		// Check for variable block start
		if !inBlock && hasPrefix(line, "variable") {
			// Extract variable name from: variable "name" {
			if qStart := indexByte(line, '"'); qStart >= 0 {
				if qEnd := indexByte(line[qStart+1:], '"'); qEnd >= 0 {
					currentVar = line[qStart+1 : qStart+1+qEnd]
					inBlock = true
					braceDepth = 1
				}
			}
			continue
		}

		if inBlock {
			// Count braces
			for _, c := range line {
				if c == '{' {
					braceDepth++
				} else if c == '}' {
					braceDepth--
				}
			}

			// Look for default
			if hasPrefix(line, "default") {
				// Extract default value
				if eqIdx := indexByte(line, '='); eqIdx >= 0 {
					valPart := trimSpace(line[eqIdx+1:])
					val := parseSimpleValue(valPart)
					if val != nil {
						vars[currentVar] = val
					}
				}
			}

			if braceDepth == 0 {
				inBlock = false
				currentVar = ""
			}
		}
	}

	return vars, nil
}

// parseSimpleValue parses a simple HCL value
func parseSimpleValue(s string) any {
	s = trimSpace(s)

	// String
	if len(s) > 0 && s[0] == '"' {
		end := 1
		for end < len(s) && s[end] != '"' {
			end++
		}
		if end < len(s) {
			return s[1:end]
		}
	}

	// Number
	if len(s) > 0 && (s[0] >= '0' && s[0] <= '9' || s[0] == '-') {
		// Try int
		var n int64
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
			return n
		}
	}

	// Bool
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// List
	if len(s) > 0 && s[0] == '[' {
		return parseSimpleList(s)
	}

	return nil
}

// parseSimpleList parses a simple HCL list
func parseSimpleList(s string) []string {
	// Remove brackets
	s = trimSpace(s)
	if len(s) < 2 {
		return nil
	}
	if s[0] == '[' {
		s = s[1:]
	}
	// Find closing bracket
	depth := 0
	end := 0
	for i, c := range s {
		if c == '[' {
			depth++
		} else if c == ']' {
			if depth == 0 {
				end = i
				break
			}
			depth--
		}
	}
	if end > 0 {
		s = s[:end]
	}

	// Split by commas and extract strings
	var result []string
	for _, part := range splitByComma(s) {
		part = trimSpace(part)
		if len(part) > 1 && part[0] == '"' {
			qEnd := indexByte(part[1:], '"')
			if qEnd >= 0 {
				result = append(result, part[1:1+qEnd])
			}
		}
	}
	return result
}

// Helper functions to avoid importing strings for consistency
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func indexString(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func splitByComma(s string) []string {
	var result []string
	start := 0
	inQuote := false
	for i := 0; i < len(s); i++ {
		if s[i] == '"' && (i == 0 || s[i-1] != '\\') {
			inQuote = !inQuote
		}
		if s[i] == ',' && !inQuote {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
