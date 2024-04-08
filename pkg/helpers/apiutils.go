package helpers

import (
	"strings"
)

// ValidRequestHeader checks if the provided header matches the desired content type.
// It takes a headerProvider interface as a parameter and returns a boolean value.
// The function compares the "Content-Type" header value with the desired type "application/json".
// If the header matches or is empty, it returns true. Otherwise, it returns false.
func ValidRequestHeader(p headerProvider) bool {
	wantedtype := "application/json"
	headertype := p.Get("Content-Type")

	if headertype != "" {
		if strings.Contains(headertype, wantedtype) {
			return true
		} else if p.Get("Content-Type") != "application/json" {
			return false
		}
	}
	return false
}

// headerProvider is an interface that defines a method for retrieving header values.
type headerProvider interface {
	Get(string, ...string) string
}
