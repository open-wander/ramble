package resource

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCanonicalTags(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		wantErr bool
		wantMsg string
		noHint  bool
	}{
		{name: "nil", tags: nil},
		{name: "empty", tags: []string{}},
		{name: "canonical", tags: []string{"db", "ci-cd"}},
		{name: "uppercase", tags: []string{"DB"}, wantErr: true, wantMsg: `use "db"`},
		{name: "too short", tags: []string{"a"}, wantErr: true},
		{name: "comma", tags: []string{"a,b"}, wantErr: true},
		{name: "space", tags: []string{"bad tag"}, wantErr: true},
		{name: "too long", tags: []string{strings.Repeat("a", 21)}, wantErr: true},
		{name: "too long uppercase", tags: []string{strings.Repeat("A", 21)}, wantErr: true, wantMsg: "2-20"},
		{name: "duplicate", tags: []string{"db", "db"}, wantErr: true, wantMsg: "duplicated"},
		{name: "empty string", tags: []string{""}, wantErr: true},
		{name: "uppercase still invalid", tags: []string{"Bad Tag"}, wantErr: true, noHint: true},
		{name: "unicode", tags: []string{"ÉÉ"}, wantErr: true, noHint: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCanonicalTags(tt.tags)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrInvalidTags)
			if tt.wantMsg != "" {
				assert.Contains(t, err.Error(), tt.wantMsg)
			}
			if tt.noHint {
				assert.NotContains(t, err.Error(), "use ")
			}
		})
	}
}

func TestValidateCanonicalTags_LongTagIsTruncatedInMessage(t *testing.T) {
	err := ValidateCanonicalTags([]string{strings.Repeat("x", 5000)})
	require.ErrorIs(t, err, ErrInvalidTags)
	assert.Less(t, len(err.Error()), 200)
}

func TestValidateCanonicalTags_IsNotAnInputError(t *testing.T) {
	err := ValidateCanonicalTags([]string{"DB"})
	assert.False(t, IsInputError(err))
}
