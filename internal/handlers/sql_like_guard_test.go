package handlers

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	likeParamRe  = regexp.MustCompile(`(?i)\bI?LIKE\s+\?`)
	likeEscapeRe = regexp.MustCompile(`(?i)^\s*ESCAPE\b`)
)

// TestNoUnescapedLikeQueries fails on any LIKE/ILIKE with a bound parameter
// that is not followed by ESCAPE. Exact-name lookups must use iEq; only
// escaped pattern searches may use LIKE.
func TestNoUnescapedLikeQueries(t *testing.T) {
	scanned := 0
	var offenders []string

	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		text := string(src)
		for _, m := range likeParamRe.FindAllStringIndex(text, -1) {
			if likeEscapeRe.MatchString(text[m[1]:]) {
				continue
			}
			line := 1 + strings.Count(text[:m[0]], "\n")
			offenders = append(offenders, path+":"+strconv.Itoa(line))
		}
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, scanned, "guard scanned no Go files")
	assert.Empty(t, offenders, "LIKE/ILIKE ? without ESCAPE; use iEq for exact matches")
}
