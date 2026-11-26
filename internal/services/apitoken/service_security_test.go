//go:build security

package apitoken

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The plaintext token must not appear in any column of the stored row.
func TestPlaintextNeverStored(t *testing.T) {
	svc := New(testDB)
	u := mkUser(t, "alice")
	plain, tok, err := svc.Create(u.ID, "ci", "write", nil, 10)
	require.NoError(t, err)

	rows, err := testDB.Raw("SELECT * FROM api_tokens WHERE id = ?", tok.ID).Rows()
	require.NoError(t, err)
	defer rows.Close()
	cols, err := rows.Columns()
	require.NoError(t, err)
	require.True(t, rows.Next())
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	require.NoError(t, rows.Scan(ptrs...))
	// Body of the token (after the fixed prefix) must not appear either.
	body := plain[len(TokenPrefix):]
	for i, v := range vals {
		s := fmt.Sprint(v)
		if b, ok := v.([]byte); ok {
			s = string(b)
		}
		require.False(t, strings.Contains(s, plain), "column %s holds plaintext", cols[i])
		require.False(t, strings.Contains(s, body), "column %s holds token body", cols[i])
	}
}
