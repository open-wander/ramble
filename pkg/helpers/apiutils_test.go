package helpers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidRequestHeader(t *testing.T) {
	tests := map[string]struct {
		Value          string
		ExpectedResult bool
	}{
		"json": {
			Value:          "application/json",
			ExpectedResult: true,
		},
		"empty": {
			Value:          "",
			ExpectedResult: false,
		},
		"bad": {
			Value:          "fibble",
			ExpectedResult: false,
		},
	}
	for name, tc := range tests {
		name := name
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			provider := &testHeaderProvider{value: tc.Value}
			// Act
			got := ValidRequestHeader(provider)
			// Assert
			require.Equal(t, tc.ExpectedResult, got)
		})
	}
}

type testHeaderProvider struct {
	value string
}

func (h *testHeaderProvider) Get(_ string, _ ...string) string {
	return h.value
}
