package handlers

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadLimited_OversizedBody verifies that readLimited rejects a body
// larger than MaxRemoteFileBytes and returns an error with empty content.
func TestReadLimited_OversizedBody(t *testing.T) {
	body := bytes.Repeat([]byte("x"), MaxRemoteFileBytes+1)
	r := io.NopCloser(bytes.NewReader(body))

	content, err := readLimited(r)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum allowed size",
		"error should describe the size violation")
	assert.Empty(t, content, "oversized download must not return partial content")
}

// TestReadLimited_ExactLimit verifies that a body of exactly MaxRemoteFileBytes
// is accepted and returned in full.
func TestReadLimited_ExactLimit(t *testing.T) {
	body := bytes.Repeat([]byte("y"), MaxRemoteFileBytes)
	r := io.NopCloser(bytes.NewReader(body))

	content, err := readLimited(r)

	require.NoError(t, err)
	assert.Equal(t, MaxRemoteFileBytes, len(content),
		"body at the exact limit must be accepted in full")
}

// TestMaxRemoteFileBytes_Value verifies the constant is exactly 1 MiB.
func TestMaxRemoteFileBytes_Value(t *testing.T) {
	const oneMiB = 1 << 20
	assert.Equal(t, oneMiB, MaxRemoteFileBytes,
		"MaxRemoteFileBytes must be exactly 1 MiB (1<<20 = %d)", oneMiB)
}

// TestDownloadFile_SizeError_IsPermanent verifies that the size-limit error
// is classified as permanent so retries are skipped for consistently oversized
// responses.
func TestDownloadFile_SizeError_IsPermanent(t *testing.T) {
	// The size-limit error message contains "blocked" so that isPermanentError
	// classifies it as permanent, preventing costly retries against a server
	// that will always return an oversized response.
	sizeErr := fmt.Errorf("remote file blocked: exceeds maximum allowed size of %d bytes", MaxRemoteFileBytes)
	assert.True(t, isPermanentError(sizeErr),
		"size-limit error must be treated as permanent to prevent unnecessary retries")
}

// TestReadLimited_SmallBody verifies normal small bodies pass through unchanged.
func TestReadLimited_SmallBody(t *testing.T) {
	const payload = "hello, world"
	r := io.NopCloser(strings.NewReader(payload))

	content, err := readLimited(r)

	require.NoError(t, err)
	assert.Equal(t, payload, content)
}
