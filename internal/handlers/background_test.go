package handlers

import (
	"errors"
	"testing"
)

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "404 status code",
			err:      errors.New("HTTP 404 not found"),
			expected: true,
		},
		{
			name:     "not found message",
			err:      errors.New("file Not Found on server"),
			expected: true,
		},
		{
			name:     "403 status code",
			err:      errors.New("HTTP 403 forbidden"),
			expected: true,
		},
		{
			name:     "401 status code",
			err:      errors.New("HTTP 401 unauthorized"),
			expected: true,
		},
		{
			name:     "SSRF error",
			err:      errors.New("SSRF attempt blocked"),
			expected: true,
		},
		{
			name:     "blocked error",
			err:      errors.New("request blocked by security policy"),
			expected: true,
		},
		{
			name:     "invalid URL",
			err:      errors.New("invalid URL format"),
			expected: true,
		},
		{
			name:     "malformed error",
			err:      errors.New("malformed request"),
			expected: true,
		},
		{
			name:     "connection refused - transient",
			err:      errors.New("connection refused"),
			expected: false,
		},
		{
			name:     "timeout - transient",
			err:      errors.New("request timeout"),
			expected: false,
		},
		{
			name:     "status 500 - transient",
			err:      errors.New("status 500 internal server error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPermanentError(tt.err)
			if result != tt.expected {
				t.Errorf("isPermanentError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}
