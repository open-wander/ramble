package render

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// TemplateFuncs returns the template function map for pack rendering
func TemplateFuncs(ctx *RenderContext) map[string]any {
	return map[string]any{
		// Variable access - returns variable value from context
		"var": func(name string, _ any) any {
			if val, ok := ctx.Variables[name]; ok {
				return val
			}
			return nil
		},

		// Metadata access - returns pack metadata by path
		"meta": func(path string, _ any) any {
			parts := strings.Split(path, ".")
			if len(parts) < 2 {
				return nil
			}

			switch parts[0] {
			case "pack":
				switch parts[1] {
				case "name":
					return ctx.PackName
				case "description":
					return ctx.PackDescription
				case "version":
					return ctx.PackVersion
				}
			}
			return nil
		},

		// String functions
		"quote": func(v any) string {
			return fmt.Sprintf("%q", toString(v))
		},

		"toStringList": toStringList,
		"coalesce":     coalesce,
		"indent":       indent,
		"nindent":      nindent,

		// Type conversion
		"toJSON":       toJSON,
		"toPrettyJSON": toPrettyJSON,
	}
}

// toStringList converts a value to an HCL-style string list
func toStringList(v any) string {
	if v == nil {
		return "[]"
	}

	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Slice, reflect.Array:
		items := make([]string, val.Len())
		for i := 0; i < val.Len(); i++ {
			items[i] = fmt.Sprintf("%q", toString(val.Index(i).Interface()))
		}
		return "[" + strings.Join(items, ", ") + "]"
	case reflect.String:
		// Single string - wrap in list
		return fmt.Sprintf("[%q]", val.String())
	default:
		return "[]"
	}
}

// coalesce returns the first non-empty value
func coalesce(values ...any) any {
	for _, v := range values {
		if v == nil {
			continue
		}
		val := reflect.ValueOf(v)
		switch val.Kind() {
		case reflect.String:
			if val.String() != "" {
				return v
			}
		case reflect.Slice, reflect.Map:
			if val.Len() > 0 {
				return v
			}
		case reflect.Ptr, reflect.Interface:
			if !val.IsNil() {
				return v
			}
		default:
			// For numbers, bools, etc. - return if not zero value
			if !val.IsZero() {
				return v
			}
		}
	}
	return nil
}

// indent adds indentation to each line
func indent(spaces int, v any) string {
	s := toString(v)
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// nindent adds a newline then indentation to each line
func nindent(spaces int, v any) string {
	return "\n" + indent(spaces, v)
}

// toJSON converts value to JSON string
func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// toPrettyJSON converts value to pretty-printed JSON
func toPrettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

// toString converts any value to string
func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
