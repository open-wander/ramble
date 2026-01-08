package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToStringList(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: "[]",
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: "[]",
		},
		{
			name:     "single string",
			input:    []string{"hello"},
			expected: `["hello"]`,
		},
		{
			name:     "multiple strings",
			input:    []string{"a", "b", "c"},
			expected: `["a", "b", "c"]`,
		},
		{
			name:     "strings with special chars",
			input:    []string{"hello world", "foo\"bar"},
			expected: `["hello world", "foo\"bar"]`,
		},
		{
			name:     "interface slice",
			input:    []interface{}{"x", "y"},
			expected: `["x", "y"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toStringList(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCoalesce(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []any
		expected any
	}{
		{
			name:     "first non-empty string",
			inputs:   []any{"", "hello", "world"},
			expected: "hello",
		},
		{
			name:     "all empty returns nil",
			inputs:   []any{"", "", nil},
			expected: nil,
		},
		{
			name:     "first value is non-empty",
			inputs:   []any{"first", "second"},
			expected: "first",
		},
		{
			name:     "nil then value",
			inputs:   []any{nil, "value"},
			expected: "value",
		},
		{
			name:     "number zero is empty",
			inputs:   []any{0, 42},
			expected: 42,
		},
		{
			name:     "bool false is empty",
			inputs:   []any{false, true},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := coalesce(tt.inputs...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIndent(t *testing.T) {
	tests := []struct {
		name     string
		spaces   int
		input    string
		expected string
	}{
		{
			name:     "single line",
			spaces:   2,
			input:    "hello",
			expected: "  hello",
		},
		{
			name:     "multiple lines",
			spaces:   4,
			input:    "line1\nline2",
			expected: "    line1\n    line2",
		},
		{
			name:     "empty string",
			spaces:   2,
			input:    "",
			expected: "",
		},
		{
			name:     "preserve empty lines",
			spaces:   2,
			input:    "a\n\nb",
			expected: "  a\n\n  b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := indent(tt.spaces, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNindent(t *testing.T) {
	result := nindent(2, "hello")
	assert.Equal(t, "\n  hello", result)
}

func TestToJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "string",
			input:    "hello",
			expected: `"hello"`,
		},
		{
			name:     "number",
			input:    42,
			expected: "42",
		},
		{
			name:     "map",
			input:    map[string]int{"a": 1},
			expected: `{"a":1}`,
		},
		{
			name:     "slice",
			input:    []string{"a", "b"},
			expected: `["a","b"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFuncs(t *testing.T) {
	ctx := &RenderContext{
		Variables: map[string]any{
			"name":  "test-app",
			"count": 3,
		},
		PackName:        "my-pack",
		PackDescription: "A test pack",
		PackVersion:     "1.0.0",
	}

	funcs := TemplateFuncs(ctx)

	t.Run("var function returns variable", func(t *testing.T) {
		varFunc := funcs["var"].(func(string, any) any)
		assert.Equal(t, "test-app", varFunc("name", nil))
		assert.Equal(t, 3, varFunc("count", nil))
		assert.Nil(t, varFunc("nonexistent", nil))
	})

	t.Run("meta function returns pack metadata", func(t *testing.T) {
		metaFunc := funcs["meta"].(func(string, any) any)
		assert.Equal(t, "my-pack", metaFunc("pack.name", nil))
		assert.Equal(t, "A test pack", metaFunc("pack.description", nil))
		assert.Equal(t, "1.0.0", metaFunc("pack.version", nil))
		assert.Nil(t, metaFunc("invalid.path", nil))
	})

	t.Run("quote function quotes strings", func(t *testing.T) {
		quoteFunc := funcs["quote"].(func(any) string)
		assert.Equal(t, `"hello"`, quoteFunc("hello"))
		assert.Equal(t, `"42"`, quoteFunc(42))
	})
}
