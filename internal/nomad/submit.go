package nomad

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

// SubmitJob submits a job to Nomad by shelling out to the nomad binary
func SubmitJob(jobHCL string, addr string) error {
	// Write job to temp file
	tmpFile, err := os.CreateTemp("", "ramble-job-*.nomad.hcl")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(jobHCL); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write job file: %w", err)
	}
	tmpFile.Close()

	// Build nomad command
	args := []string{"job", "run", tmpFile.Name()}
	cmd := exec.Command("nomad", args...)

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
		tmpFile.Close()
		return fmt.Errorf("failed to write job file: %w", err)
	}
	tmpFile.Close()

	// Build nomad validate command
	args := []string{"job", "validate", tmpFile.Name()}
	cmd := exec.Command("nomad", args...)

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
		tmpFile.Close()
		return "", fmt.Errorf("failed to write job file: %w", err)
	}
	tmpFile.Close()

	// Build nomad plan command
	args := []string{"job", "plan", tmpFile.Name()}
	cmd := exec.Command("nomad", args...)

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
	cmd.Run()

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
