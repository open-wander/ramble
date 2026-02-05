package nomad

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// validateTempFilePath ensures the file path is within the system temp directory
// to prevent path injection attacks
func validateTempFilePath(path string) error {
	cleanPath := filepath.Clean(path)
	tempDir := filepath.Clean(os.TempDir())
	if !strings.HasPrefix(cleanPath, tempDir+string(os.PathSeparator)) {
		return fmt.Errorf("file path %q is outside temp directory", path)
	}
	return nil
}

// SubmitJob submits a job to Nomad by shelling out to the nomad binary
func SubmitJob(jobHCL string, addr string) error {
	// Write job to temp file
	tmpFile, err := os.CreateTemp("", "ramble-job-*.nomad.hcl")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(jobHCL); err != nil {
		_ = tmpFile.Close() // Close on write error, ignore close error
		return fmt.Errorf("failed to write job file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close job file: %w", err)
	}

	// Validate temp file path to prevent command injection
	tmpPath := filepath.Clean(tmpFile.Name())
	if err := validateTempFilePath(tmpPath); err != nil {
		return err
	}

	// Build nomad command with validated path
	// G204: Subprocess uses hardcoded "nomad" executable and validated temp file path
	cmd := exec.Command("nomad", "job", "run", tmpPath) //#nosec G204 -- executable is hardcoded, path is validated temp file

	// Set environment
	cmd.Env = os.Environ()
	if addr != "" {
		cmd.Env = append(cmd.Env, "NOMAD_ADDR="+addr)
	}

	// Connect stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nomad job run failed: %w", err)
	}

	return nil
}

// ValidateJob validates a job without submitting it
func ValidateJob(jobHCL string, addr string) error {
	// Write job to temp file
	tmpFile, err := os.CreateTemp("", "ramble-job-*.nomad.hcl")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(jobHCL); err != nil {
		_ = tmpFile.Close() // Close on write error, ignore close error
		return fmt.Errorf("failed to write job file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close job file: %w", err)
	}

	// Validate temp file path to prevent command injection
	tmpPath := filepath.Clean(tmpFile.Name())
	if err := validateTempFilePath(tmpPath); err != nil {
		return err
	}

	// Build nomad validate command with validated path
	// G204: Subprocess uses hardcoded "nomad" executable and validated temp file path
	cmd := exec.Command("nomad", "job", "validate", tmpPath) //#nosec G204 -- executable is hardcoded, path is validated temp file

	// Set environment
	cmd.Env = os.Environ()
	if addr != "" {
		cmd.Env = append(cmd.Env, "NOMAD_ADDR="+addr)
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("validation failed: %s", stderr.String())
	}

	return nil
}

// PlanJob runs nomad job plan to show what would change
func PlanJob(jobHCL string, addr string) (string, error) {
	// Write job to temp file
	tmpFile, err := os.CreateTemp("", "ramble-job-*.nomad.hcl")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(jobHCL); err != nil {
		_ = tmpFile.Close() // Close on write error, ignore close error
		return "", fmt.Errorf("failed to write job file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close job file: %w", err)
	}

	// Validate temp file path to prevent command injection
	tmpPath := filepath.Clean(tmpFile.Name())
	if err := validateTempFilePath(tmpPath); err != nil {
		return "", err
	}

	// Build nomad plan command with validated path
	// G204: Subprocess uses hardcoded "nomad" executable and validated temp file path
	cmd := exec.Command("nomad", "job", "plan", tmpPath) //#nosec G204 -- executable is hardcoded, path is validated temp file

	// Set environment
	cmd.Env = os.Environ()
	if addr != "" {
		cmd.Env = append(cmd.Env, "NOMAD_ADDR="+addr)
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run (plan exits 1 if there are changes, which is not an error)
	// Ignore the error from Run() since exit code 1 is normal for plan with changes
	_ = cmd.Run()

	if stderr.Len() > 0 {
		return "", fmt.Errorf("plan failed: %s", stderr.String())
	}

	return stdout.String(), nil
}

// CheckNomadAvailable verifies that the nomad binary is available
func CheckNomadAvailable() error {
	_, err := exec.LookPath("nomad")
	if err != nil {
		return fmt.Errorf("nomad binary not found in PATH")
	}
	return nil
}
