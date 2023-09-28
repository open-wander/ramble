package helpers

import (
	"strings"
)

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

type headerProvider interface {
	Get(string, ...string) string
}
