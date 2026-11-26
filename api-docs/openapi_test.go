package api_docs

import (
	"context"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const errorSchemaRef = "#/components/schemas/handlers.APIErrorResponse"

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("openapi.yaml")
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))
	return doc
}

func TestOpenAPISpecValid(t *testing.T) {
	doc := loadSpec(t)
	assert.True(t, strings.HasPrefix(doc.OpenAPI, "3.0."), doc.OpenAPI)
}

func TestOpenAPIOperations(t *testing.T) {
	doc := loadSpec(t)

	const minOperations = 10
	seen := map[string]string{}
	for path, item := range doc.Paths.Map() {
		assert.Contains(t, strings.Split(path, "/"), "v1", "non-v1 path in spec: %s", path)
		for method, op := range item.Operations() {
			where := method + " " + path
			require.NotEmpty(t, op.OperationID, where)
			if prev, dup := seen[op.OperationID]; dup {
				t.Errorf("operationId %q used by %s and %s", op.OperationID, prev, where)
			}
			seen[op.OperationID] = where

			for code, resp := range op.Responses.Map() {
				if code[0] != '4' && code[0] != '5' {
					continue
				}
				content := resp.Value.Content.Get("application/json")
				if assert.NotNil(t, content, "%s %s has no JSON body", where, code) {
					assert.Equal(t, errorSchemaRef, content.Schema.Ref, "%s %s", where, code)
				}
			}
		}
	}
	assert.GreaterOrEqual(t, len(seen), minOperations)
}

func TestOpenAPIBearerScheme(t *testing.T) {
	doc := loadSpec(t)
	ref := doc.Components.SecuritySchemes["bearerAuth"]
	require.NotNil(t, ref)
	assert.Equal(t, "http", ref.Value.Type)
	assert.Equal(t, "bearer", ref.Value.Scheme)
}
