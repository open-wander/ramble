package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

const (
	openAPIVersion = "3.0.3"
	bearerScheme   = "bearerAuth"
)

// Convert turns the Swagger 2.0 document produced by swag into an OpenAPI 3
// document in YAML. Only the JSON API (paths with a "v1" segment) is kept, the
// bearerAuth apiKey definition becomes an http bearer scheme, and the output is
// byte-stable for a given input.
func Convert(swaggerJSON []byte) ([]byte, error) {
	var doc2 openapi2.T
	if err := json.Unmarshal(swaggerJSON, &doc2); err != nil {
		return nil, fmt.Errorf("parse swagger 2.0: %w", err)
	}

	def := doc2.SecurityDefinitions[bearerScheme]
	if def == nil {
		return nil, fmt.Errorf("security definition %q is missing", bearerScheme)
	}
	if def.Type != "apiKey" || def.In != "header" || def.Name != "Authorization" {
		return nil, fmt.Errorf("security definition %q must be apiKey in header Authorization, got %s/%s/%s",
			bearerScheme, def.Type, def.In, def.Name)
	}

	for p := range doc2.Paths {
		if !slices.Contains(strings.Split(p, "/"), "v1") {
			delete(doc2.Paths, p)
		}
	}

	doc3, err := openapi2conv.ToV3(&doc2)
	if err != nil {
		return nil, fmt.Errorf("convert to openapi 3: %w", err)
	}
	doc3.OpenAPI = openAPIVersion

	if doc3.Components == nil {
		return nil, errors.New("converted document has no components")
	}
	doc3.Components.SecuritySchemes[bearerScheme] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type:         "http",
			Scheme:       "bearer",
			Description:  def.Description,
			BearerFormat: "token",
		},
	}

	if err := doc3.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("validate openapi 3: %w", err)
	}

	jsonBytes, err := json.Marshal(doc3)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	// Round-trip through generic maps: yaml.v3 sorts map keys.
	var generic any
	if err := yaml.Unmarshal(jsonBytes, &generic); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	return yaml.Marshal(generic)
}
