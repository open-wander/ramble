package main

import (
	"fmt"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const baseDoc = `{
  "swagger": "2.0",
  "info": {"title": "t", "version": "1"},
  "securityDefinitions": %s,
  "paths": {
    "/v1/packs": {"get": {"operationId": "listPacks", "produces": ["application/json"],
      "responses": {"200": {"description": "ok", "schema": {"type": "object"}}}}},
    "/{username}/v1/packs": {"get": {"operationId": "listNamespacePacks", "produces": ["application/json"],
      "parameters": [{"type": "string", "name": "username", "in": "path", "required": true}],
      "responses": {"200": {"description": "ok", "schema": {"type": "object"}}}}},
    "/registries": {"get": {"operationId": "htmlRegistries", "produces": ["text/html"],
      "responses": {"200": {"description": "ok", "schema": {"type": "string"}}}}},
    "/v1x/packs": {"get": {"operationId": "notV1Prefix", "produces": ["application/json"],
      "responses": {"200": {"description": "ok", "schema": {"type": "object"}}}}},
    "/docs/v10": {"get": {"operationId": "notV1Segment", "produces": ["text/html"],
      "responses": {"200": {"description": "ok", "schema": {"type": "string"}}}}}
  }
}`

const apiKeyDef = `{"bearerAuth": {"type": "apiKey", "in": "header", "name": "Authorization"}}`

func input(defs string) []byte { return []byte(fmt.Sprintf(baseDoc, defs)) }

func load(t *testing.T, out []byte) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(out)
	require.NoError(t, err)
	return doc
}

func TestConvert(t *testing.T) {
	out, err := Convert(input(apiKeyDef))
	require.NoError(t, err)
	doc := load(t, out)

	t.Run("version", func(t *testing.T) {
		assert.Equal(t, "3.0.3", doc.OpenAPI)
	})
	t.Run("bearer scheme", func(t *testing.T) {
		ref := doc.Components.SecuritySchemes["bearerAuth"]
		require.NotNil(t, ref)
		assert.Equal(t, "http", ref.Value.Type)
		assert.Equal(t, "bearer", ref.Value.Scheme)
	})
	t.Run("operationId preserved", func(t *testing.T) {
		op := doc.Paths.Value("/v1/packs").Get
		assert.Equal(t, "listPacks", op.OperationID)
		op = doc.Paths.Value("/{username}/v1/packs").Get
		assert.Equal(t, "listNamespacePacks", op.OperationID)
	})
	t.Run("non-v1 path dropped", func(t *testing.T) {
		assert.Nil(t, doc.Paths.Value("/registries"))
		// "v1" must be a whole path segment, not a substring.
		assert.Nil(t, doc.Paths.Value("/v1x/packs"))
		assert.Nil(t, doc.Paths.Value("/docs/v10"))
		assert.Equal(t, 2, doc.Paths.Len())
	})
	t.Run("deterministic", func(t *testing.T) {
		again, err := Convert(input(apiKeyDef))
		require.NoError(t, err)
		assert.Equal(t, string(out), string(again))
		assert.Equal(t, byte('\n'), out[len(out)-1])
	})
}

func TestConvertRejectsBadSecurity(t *testing.T) {
	tests := []struct{ name, defs string }{
		{"missing", `{}`},
		{"basic", `{"bearerAuth": {"type": "basic"}}`},
		{"query key", `{"bearerAuth": {"type": "apiKey", "in": "query", "name": "Authorization"}}`},
		{"wrong header", `{"bearerAuth": {"type": "apiKey", "in": "header", "name": "X-Key"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Convert(input(tt.defs))
			assert.Error(t, err)
		})
	}
}

func TestConvertRejectsBadJSON(t *testing.T) {
	_, err := Convert([]byte("{"))
	assert.Error(t, err)
}
