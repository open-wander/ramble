// Command openapi-convert turns the swag-generated Swagger 2.0 document into
// the OpenAPI 3 spec of the /v1 API. It is a development tool and is not part
// of the shipped binaries.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	in := flag.String("in", "api-docs/swagger.json", "Swagger 2.0 input")
	out := flag.String("out", "api-docs/openapi.yaml", "OpenAPI 3 output")
	flag.Parse()

	if err := run(*in, *out); err != nil {
		fmt.Fprintln(os.Stderr, "openapi-convert:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	// Trusted repo paths passed on the command line by the Makefile.
	data, err := os.ReadFile(filepath.Clean(in)) // #nosec G304 -- dev tool, repo path
	if err != nil {
		return err
	}
	converted, err := Convert(data)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(out), converted, 0o644) // #nosec G306 -- spec is a public artifact
}
