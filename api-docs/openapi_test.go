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

func TestOpenAPICurrentUser(t *testing.T) {
	doc := loadSpec(t)
	item := doc.Paths.Find("/v1/user")
	require.NotNil(t, item)
	op := item.Get
	require.NotNil(t, op)
	assert.Equal(t, "getCurrentUser", op.OperationID)

	require.NotNil(t, op.Security)
	var found bool
	for _, req := range *op.Security {
		if _, ok := req["bearerAuth"]; ok {
			found = true
		}
	}
	assert.True(t, found, "bearerAuth security")

	r401 := op.Responses.Value("401")
	require.NotNil(t, r401)
	assert.Equal(t, errorSchemaRef, r401.Value.Content.Get("application/json").Schema.Ref)

	schema := doc.Components.Schemas["handlers.CurrentUserResponse"]
	require.NotNil(t, schema)
	org := schema.Value.Properties["token_org"]
	require.NotNil(t, org)
	assert.True(t, org.Value.Nullable, "token_org nullable")
	assert.ElementsMatch(t, []string{"username", "token_scope", "token_org", "token_expires_at"}, schema.Value.Required)
}

func TestOpenAPICreateResource(t *testing.T) {
	doc := loadSpec(t)
	item := doc.Paths.Find("/v1/resources")
	require.NotNil(t, item)
	op := item.Post
	require.NotNil(t, op)
	assert.Equal(t, "createResource", op.OperationID)

	require.NotNil(t, op.Security)
	var found bool
	for _, req := range *op.Security {
		if _, ok := req["bearerAuth"]; ok {
			found = true
		}
	}
	assert.True(t, found, "bearerAuth security")

	require.NotNil(t, op.RequestBody)
	body := op.RequestBody.Value.Content.Get("application/json")
	require.NotNil(t, body)
	assert.Equal(t, "#/components/schemas/handlers.CreateResourceRequest", body.Schema.Ref)

	created := op.Responses.Value("201")
	require.NotNil(t, created)
	assert.Equal(t, "#/components/schemas/handlers.CreateResourceResponse",
		created.Value.Content.Get("application/json").Schema.Ref)
	for _, code := range []string{"400", "401", "403", "404", "409", "422", "429", "500"} {
		r := op.Responses.Value(code)
		if assert.NotNil(t, r, code) {
			assert.Equal(t, errorSchemaRef, r.Value.Content.Get("application/json").Schema.Ref, code)
		}
	}

	req := doc.Components.Schemas["handlers.CreateResourceRequest"]
	require.NotNil(t, req)
	assert.ElementsMatch(t, []string{"name", "type", "namespace", "repository_url", "version"}, req.Value.Required)

	resp := doc.Components.Schemas["handlers.CreateResourceResponse"]
	require.NotNil(t, resp)
	assert.Contains(t, resp.Value.Required, "webhook_secret")
	assert.Contains(t, resp.Value.Properties, "latest_version")
	assert.Empty(t, resp.Value.AllOf, "embedded ResourceResponse must be flattened")
}

func TestOpenAPIGetResource(t *testing.T) {
	doc := loadSpec(t)
	for _, tt := range []struct {
		path   string
		opID   string
		params []string
	}{
		{"/v1/resources/{id}", "getResource", []string{"id"}},
		{"/v1/namespaces/{namespace}/resources/{name}", "getResourceByName", []string{"namespace", "name"}},
	} {
		t.Run(tt.opID, func(t *testing.T) {
			item := doc.Paths.Find(tt.path)
			require.NotNil(t, item)
			op := item.Get
			require.NotNil(t, op)
			assert.Equal(t, tt.opID, op.OperationID)

			require.NotNil(t, op.Security)
			var found bool
			for _, req := range *op.Security {
				if _, ok := req["bearerAuth"]; ok {
					found = true
				}
			}
			assert.True(t, found, "bearerAuth security")

			var names []string
			for _, p := range op.Parameters {
				assert.Equal(t, "path", p.Value.In)
				assert.True(t, p.Value.Required)
				names = append(names, p.Value.Name)
			}
			assert.ElementsMatch(t, tt.params, names)

			ok := op.Responses.Value("200")
			require.NotNil(t, ok)
			assert.Equal(t, "#/components/schemas/handlers.ResourceResponse",
				ok.Value.Content.Get("application/json").Schema.Ref)
			for _, code := range []string{"401", "403", "404", "500"} {
				r := op.Responses.Value(code)
				if assert.NotNil(t, r, code) {
					assert.Equal(t, errorSchemaRef, r.Value.Content.Get("application/json").Schema.Ref, code)
				}
			}
		})
	}

	resp := doc.Components.Schemas["handlers.ResourceResponse"]
	require.NotNil(t, resp)
	assert.NotContains(t, resp.Value.Properties, "webhook_secret")
	assert.Contains(t, resp.Value.Properties, "latest_version")
}

func hasBearer(op *openapi3.Operation) bool {
	if op.Security == nil {
		return false
	}
	for _, req := range *op.Security {
		if _, ok := req["bearerAuth"]; ok {
			return true
		}
	}
	return false
}

func TestOpenAPIUpdateResource(t *testing.T) {
	doc := loadSpec(t)
	item := doc.Paths.Find("/v1/resources/{id}")
	require.NotNil(t, item)
	op := item.Patch
	require.NotNil(t, op)
	assert.Equal(t, "updateResource", op.OperationID)
	assert.True(t, hasBearer(op), "bearerAuth security")

	require.Len(t, op.Parameters, 1)
	assert.Equal(t, "id", op.Parameters[0].Value.Name)
	assert.Equal(t, "path", op.Parameters[0].Value.In)

	require.NotNil(t, op.RequestBody)
	body := op.RequestBody.Value.Content.Get("application/json")
	require.NotNil(t, body)
	assert.Equal(t, "#/components/schemas/handlers.UpdateResourceRequest", body.Schema.Ref)

	ok := op.Responses.Value("200")
	require.NotNil(t, ok)
	assert.Equal(t, "#/components/schemas/handlers.ResourceResponse", ok.Value.Content.Get("application/json").Schema.Ref)
	for _, code := range []string{"400", "401", "403", "404", "422", "429", "500"} {
		r := op.Responses.Value(code)
		if assert.NotNil(t, r, code) {
			assert.Equal(t, errorSchemaRef, r.Value.Content.Get("application/json").Schema.Ref, code)
		}
	}

	req := doc.Components.Schemas["handlers.UpdateResourceRequest"]
	require.NotNil(t, req)
	assert.Empty(t, req.Value.Required, "every field is optional")
	assert.ElementsMatch(t, []string{"description", "repository_url", "file_path", "license", "tags"}, propNames(req.Value))
}

func propNames(s *openapi3.Schema) []string {
	var out []string
	for k := range s.Properties {
		out = append(out, k)
	}
	return out
}

func TestOpenAPIDeleteResource(t *testing.T) {
	doc := loadSpec(t)
	item := doc.Paths.Find("/v1/resources/{id}")
	require.NotNil(t, item)
	op := item.Delete
	require.NotNil(t, op)
	assert.Equal(t, "deleteResource", op.OperationID)
	assert.True(t, hasBearer(op), "bearerAuth security")
	assert.Nil(t, op.RequestBody)

	done := op.Responses.Value("204")
	require.NotNil(t, done)
	assert.Empty(t, done.Value.Content, "204 has no body")
	assert.Nil(t, op.Responses.Value("200"))
	for _, code := range []string{"401", "403", "404", "429", "500"} {
		r := op.Responses.Value(code)
		if assert.NotNil(t, r, code) {
			assert.Equal(t, errorSchemaRef, r.Value.Content.Get("application/json").Schema.Ref, code)
		}
	}
}

func TestOpenAPIVersionAndWebhookOperations(t *testing.T) {
	doc := loadSpec(t)
	const versionSchema = "#/components/schemas/handlers.ResourceVersionResponse"
	tests := []struct {
		path, method, id, okCode string
		params                   []string
		body                     string
		okRef                    string
		errCodes                 []string
	}{
		{"/v1/resources/{id}/versions", "GET", "listResourceVersions", "200", []string{"id"}, "", "", []string{"401", "403", "404", "500"}},
		{"/v1/resources/{id}/versions", "POST", "createResourceVersion", "202", []string{"id"}, "#/components/schemas/handlers.CreateVersionRequest", versionSchema, []string{"400", "401", "403", "404", "409", "422", "429", "500"}},
		{"/v1/resources/{id}/versions/{version}/retry", "POST", "retryResourceVersion", "202", []string{"id", "version"}, "", versionSchema, []string{"401", "403", "404", "409", "429", "500"}},
		{"/v1/resources/{id}/webhook-secret", "POST", "rotateWebhookSecret", "200", []string{"id"}, "", "#/components/schemas/handlers.WebhookSecretResponse", []string{"401", "403", "404", "429", "500"}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			item := doc.Paths.Find(tt.path)
			require.NotNil(t, item)
			op := item.GetOperation(tt.method)
			require.NotNil(t, op)
			assert.Equal(t, tt.id, op.OperationID)
			assert.True(t, hasBearer(op), "bearerAuth security")

			var params []string
			for _, p := range op.Parameters {
				assert.Equal(t, "path", p.Value.In)
				params = append(params, p.Value.Name)
			}
			assert.Equal(t, tt.params, params)

			if tt.body == "" {
				assert.Nil(t, op.RequestBody)
			} else {
				require.NotNil(t, op.RequestBody)
				assert.Equal(t, tt.body, op.RequestBody.Value.Content.Get("application/json").Schema.Ref)
			}
			ok := op.Responses.Value(tt.okCode)
			require.NotNil(t, ok)
			schema := ok.Value.Content.Get("application/json").Schema
			if tt.okRef != "" {
				assert.Equal(t, tt.okRef, schema.Ref)
			} else {
				assert.Equal(t, "array", schema.Value.Type.Slice()[0])
				assert.Equal(t, versionSchema, schema.Value.Items.Ref)
			}
			for _, code := range tt.errCodes {
				r := op.Responses.Value(code)
				if assert.NotNil(t, r, code) {
					assert.Equal(t, errorSchemaRef, r.Value.Content.Get("application/json").Schema.Ref, code)
				}
			}
		})
	}

	v := doc.Components.Schemas["handlers.ResourceVersionResponse"]
	require.NotNil(t, v)
	assert.ElementsMatch(t, []string{"version", "fetch_status", "fetch_error", "created_at"}, propNames(v.Value))
}
