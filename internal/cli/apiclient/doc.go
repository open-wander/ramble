// Package apiclient is the Go client for the registry /v1 API, generated from
// api-docs/openapi.yaml. Build clients with New, never with the generated
// constructors, so a token always travels over the hardened HTTP client.
package apiclient

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.9.0 -generate types,client -package apiclient -o client.gen.go ../../../api-docs/openapi.yaml
