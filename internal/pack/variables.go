package pack

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsimple"
)

// Variable represents a pack variable definition
type Variable struct {
	Name        string `hcl:"name,label" json:"name"`
	Description string `hcl:"description,optional" json:"description"`
	Type        string `hcl:"type,optional" json:"type"`
	Default     any    `json:"default"`
}

// variablesFile is the HCL structure for variables.hcl
type variablesFile struct {
	Variables []variableBlock `hcl:"variable,block"`
}

type variableBlock struct {
	Name        string         `hcl:"name,label"`
	Description string         `hcl:"description,optional"`
	Type        hcl.Expression `hcl:"type,optional"`
	Default     hcl.Expression `hcl:"default,optional"`
}

// ParseVariablesFile parses a variables.hcl file and returns variable definitions
func ParseVariablesFile(path string) ([]Variable, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read variables file: %w", err)
	}

	return ParseVariables(string(content))
}

// ParseVariables parses variable definitions from HCL content
func ParseVariables(content string) ([]Variable, error) {
	var file variablesFile
	err := hclsimple.Decode("variables.hcl", []byte(content), nil, &file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse variables: %w", err)
	}

	vars := make([]Variable, len(file.Variables))
	for i, v := range file.Variables {
		vars[i] = Variable{
			Name:        v.Name,
			Description: v.Description,
		}

		// Extract type if present
		if v.Type != nil {
			// For now, just store the type expression range text
			vars[i].Type = "string" // Default to string
		}

		// Extract default value if present
		if v.Default != nil {
			val, diags := v.Default.Value(nil)
			if !diags.HasErrors() {
				vars[i].Default = ctyValueToGo(val)
			}
		}
	}

	return vars, nil
}

// ctyValueToGo converts a cty.Value to a Go value
func ctyValueToGo(val interface{ GoString() string }) any {
	// Use GoString() to get a string representation, then parse it
	// This is a simplified approach; for production, use cty.Value type assertions
	s := val.GoString()

	// Try to detect the type from the string representation
	if s == "cty.True" {
		return true
	}
	if s == "cty.False" {
		return false
	}
	if strings.HasPrefix(s, "cty.StringVal(") {
		// Extract string value
		s = strings.TrimPrefix(s, "cty.StringVal(")
		s = strings.TrimSuffix(s, ")")
		s = strings.Trim(s, "\"")
		return s
	}
	if strings.HasPrefix(s, "cty.NumberIntVal(") {
		s = strings.TrimPrefix(s, "cty.NumberIntVal(")
		s = strings.TrimSuffix(s, ")")
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
	}
	if strings.HasPrefix(s, "cty.ListVal") || strings.HasPrefix(s, "cty.TupleVal") {
		// For lists/tuples, return as-is for now - will need proper parsing
		return []string{}
	}

	return s
}

// ExtractDefaults returns a map of variable names to their default values
func ExtractDefaults(vars []Variable) map[string]any {
	defaults := make(map[string]any)
	for _, v := range vars {
		if v.Default != nil {
			defaults[v.Name] = v.Default
		}
	}
	return defaults
}

// ParseVarFlag parses a --var key=value flag
func ParseVarFlag(s string) (key string, value any, err error) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("invalid var format: %s (expected key=value)", s)
	}

	key = parts[0]
	valStr := parts[1]

	// Try to parse as JSON for complex types
	var jsonVal any
	if err := json.Unmarshal([]byte(valStr), &jsonVal); err == nil {
		return key, jsonVal, nil
	}

	// Try to parse as number
	if n, err := strconv.ParseInt(valStr, 10, 64); err == nil {
		return key, n, nil
	}
	if f, err := strconv.ParseFloat(valStr, 64); err == nil {
		return key, f, nil
	}

	// Try to parse as bool
	if valStr == "true" {
		return key, true, nil
	}
	if valStr == "false" {
		return key, false, nil
	}

	// Return as string
	return key, valStr, nil
}

// ParseVarFile parses a variable file (HCL or JSON)
func ParseVarFile(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read var file: %w", err)
	}

	// Try JSON first
	var jsonVars map[string]any
	if err := json.Unmarshal(content, &jsonVars); err == nil {
		return jsonVars, nil
	}

	// Try HCL
	// For now, only support JSON var files
	return nil, fmt.Errorf("var file must be JSON format")
}
