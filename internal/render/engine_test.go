package render

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name      string
		template  string
		variables map[string]any
		packMeta  struct{ name, desc, version string }
		expected  string
		wantErr   bool
	}{
		{
			name:     "simple variable substitution",
			template: `name = [[ var "app_name" . | quote ]]`,
			variables: map[string]any{
				"app_name": "my-app",
			},
			expected: `name = "my-app"`,
		},
		{
			name:     "number variable",
			template: `count = [[ var "count" . ]]`,
			variables: map[string]any{
				"count": 5,
			},
			expected: `count = 5`,
		},
		{
			name:     "list variable with toStringList",
			template: `datacenters = [[ var "dcs" . | toStringList ]]`,
			variables: map[string]any{
				"dcs": []string{"dc1", "dc2"},
			},
			expected: `datacenters = ["dc1", "dc2"]`,
		},
		{
			name:     "conditional with if",
			template: `[[ if var "enabled" . ]]enabled = true[[ end ]]`,
			variables: map[string]any{
				"enabled": true,
			},
			expected: `enabled = true`,
		},
		{
			name:     "conditional false",
			template: `[[ if var "enabled" . ]]enabled = true[[ end ]]`,
			variables: map[string]any{
				"enabled": false,
			},
			expected: ``,
		},
		{
			name:     "pack metadata",
			template: `job [[ meta "pack.name" . | quote ]] {}`,
			packMeta: struct{ name, desc, version string }{
				name: "hello-world",
			},
			expected: `job "hello-world" {}`,
		},
		{
			name:     "coalesce with empty first value",
			template: `name = [[ coalesce (var "custom_name" .) (meta "pack.name" .) | quote ]]`,
			variables: map[string]any{
				"custom_name": "",
			},
			packMeta: struct{ name, desc, version string }{
				name: "default-name",
			},
			expected: `name = "default-name"`,
		},
		{
			name:     "coalesce with provided value",
			template: `name = [[ coalesce (var "custom_name" .) (meta "pack.name" .) | quote ]]`,
			variables: map[string]any{
				"custom_name": "my-custom-name",
			},
			packMeta: struct{ name, desc, version string }{
				name: "default-name",
			},
			expected: `name = "my-custom-name"`,
		},
		{
			name:     "invalid template syntax",
			template: `[[ if ]]`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine()
			engine.SetVariables(tt.variables)
			engine.SetPackMetadata(tt.packMeta.name, tt.packMeta.desc, tt.packMeta.version)

			result, err := engine.RenderTemplate(tt.template)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRenderPack(t *testing.T) {
	// Create a temporary pack directory structure
	tmpDir := t.TempDir()
	templatesDir := filepath.Join(tmpDir, "templates")
	require.NoError(t, os.MkdirAll(templatesDir, 0755))

	// Write a helper template
	helperContent := `[[- define "job_name" -]]
[[ var "job_name" . | quote ]]
[[- end -]]`
	require.NoError(t, os.WriteFile(filepath.Join(templatesDir, "_helpers.tpl"), []byte(helperContent), 0644))

	// Write a main template
	mainContent := `job [[ template "job_name" . ]] {
  count = [[ var "count" . ]]
}`
	require.NoError(t, os.WriteFile(filepath.Join(templatesDir, "main.nomad.tpl"), []byte(mainContent), 0644))

	engine := NewEngine()
	engine.SetVariables(map[string]any{
		"job_name": "my-job",
		"count":    3,
	})

	result, err := engine.RenderPack(tmpDir)
	require.NoError(t, err)

	assert.Contains(t, result, `job "my-job"`)
	assert.Contains(t, result, `count = 3`)
}

func TestRenderPackMissingTemplatesDir(t *testing.T) {
	tmpDir := t.TempDir()

	engine := NewEngine()
	_, err := engine.RenderPack(tmpDir)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "templates directory not found")
}

func TestSetVariable(t *testing.T) {
	engine := NewEngine()
	engine.SetVariable("key1", "value1")
	engine.SetVariable("key2", 42)

	result, err := engine.RenderTemplate(`[[ var "key1" . ]]-[[ var "key2" . ]]`)
	require.NoError(t, err)
	assert.Equal(t, "value1-42", result)
}
