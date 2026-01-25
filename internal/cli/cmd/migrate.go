package cmd

import (
	"encoding/base64"
	"fmt"
	"os"

	"rmbl/internal/crypto"
	"rmbl/internal/database"
	"rmbl/internal/models"

	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database migration utilities",
	Long: `Commands for running database migrations.

Examples:
  ramble migrate encrypt-tokens  Encrypt existing OAuth tokens`,
}

var (
	dryRun bool
)

var encryptTokensCmd = &cobra.Command{
	Use:   "encrypt-tokens",
	Short: "Encrypt existing OAuth tokens in the database",
	Long: `Encrypts any unencrypted OAuth access tokens stored in the database.

This command requires the TOKEN_ENCRYPTION_KEY environment variable to be set.
Generate a key with: ramble migrate generate-key

Use --dry-run to preview changes without modifying the database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check for encryption key
		keyStr := os.Getenv("TOKEN_ENCRYPTION_KEY")
		if keyStr == "" {
			return fmt.Errorf("TOKEN_ENCRYPTION_KEY environment variable not set.\nGenerate one with: ramble migrate generate-key")
		}

		// Validate key format
		key, err := base64.StdEncoding.DecodeString(keyStr)
		if err != nil {
			return fmt.Errorf("invalid TOKEN_ENCRYPTION_KEY: must be base64 encoded")
		}
		if len(key) != 32 {
			return fmt.Errorf("invalid TOKEN_ENCRYPTION_KEY: must be 32 bytes (256 bits) when decoded")
		}

		// Connect to database
		database.Connect()

		// Find users with OAuth tokens
		var users []models.User
		if err := database.DB.Where("provider != '' AND access_token != ''").Find(&users).Error; err != nil {
			return fmt.Errorf("failed to query users: %w", err)
		}

		if len(users) == 0 {
			fmt.Println("No OAuth tokens found to migrate.")
			return nil
		}

		fmt.Printf("Found %d users with OAuth tokens.\n", len(users))

		migrated := 0
		skipped := 0
		errors := 0

		for _, user := range users {
			// Check if token is already encrypted by trying to decrypt it
			// If decryption succeeds and returns different value, it was encrypted
			// If it returns the same value, it's unencrypted
			if isEncrypted(user.AccessToken) {
				skipped++
				if verbose {
					fmt.Printf("  [SKIP] User %s (ID: %d) - token already encrypted\n", user.Username, user.ID)
				}
				continue
			}

			// Encrypt the token
			encrypted, err := crypto.EncryptToken(user.AccessToken)
			if err != nil {
				errors++
				fmt.Printf("  [ERROR] User %s (ID: %d) - encryption failed: %v\n", user.Username, user.ID, err)
				continue
			}

			if dryRun {
				fmt.Printf("  [DRY-RUN] Would encrypt token for user %s (ID: %d)\n", user.Username, user.ID)
				migrated++
				continue
			}

			// Save encrypted token
			if err := database.DB.Model(&user).Update("access_token", encrypted).Error; err != nil {
				errors++
				fmt.Printf("  [ERROR] User %s (ID: %d) - save failed: %v\n", user.Username, user.ID, err)
				continue
			}

			migrated++
			if verbose {
				fmt.Printf("  [OK] User %s (ID: %d) - token encrypted\n", user.Username, user.ID)
			}
		}

		fmt.Println()
		if dryRun {
			fmt.Println("=== DRY RUN SUMMARY ===")
			fmt.Printf("Would encrypt: %d tokens\n", migrated)
		} else {
			fmt.Println("=== MIGRATION SUMMARY ===")
			fmt.Printf("Encrypted: %d tokens\n", migrated)
		}
		fmt.Printf("Skipped (already encrypted): %d tokens\n", skipped)
		if errors > 0 {
			fmt.Printf("Errors: %d\n", errors)
			return fmt.Errorf("migration completed with %d errors", errors)
		}

		return nil
	},
}

var generateKeyCmd = &cobra.Command{
	Use:   "generate-key",
	Short: "Generate a new encryption key for OAuth tokens",
	Long: `Generates a new 256-bit encryption key for encrypting OAuth tokens.

Add the generated key to your environment:
  export TOKEN_ENCRYPTION_KEY="<generated-key>"

Or add it to your .env file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		key, err := crypto.GenerateEncryptionKey()
		if err != nil {
			return fmt.Errorf("failed to generate key: %w", err)
		}

		fmt.Println("Generated encryption key (AES-256):")
		fmt.Println()
		fmt.Printf("  TOKEN_ENCRYPTION_KEY=%s\n", key)
		fmt.Println()
		fmt.Println("Add this to your environment or .env file.")
		fmt.Println("Keep this key secure - losing it means losing access to encrypted tokens.")

		return nil
	},
}

// isEncrypted checks if a token appears to be encrypted
// Encrypted tokens are base64-encoded and at least nonce (12 bytes) + some ciphertext
func isEncrypted(token string) bool {
	// Try to decode as base64
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		// Not valid base64, so it's plaintext
		return false
	}

	// AES-GCM nonce is 12 bytes, plus at least 16 bytes for auth tag
	// So minimum encrypted length is 28 bytes
	if len(data) < 28 {
		return false
	}

	// GitHub tokens start with "gho_", "ghp_", "ghu_", etc.
	// GitLab tokens start with "glpat-" or are 20+ char alphanumeric
	// If the decoded data looks like these patterns, it's not encrypted
	decoded := string(data)
	if len(decoded) > 4 {
		prefix := decoded[:4]
		if prefix == "gho_" || prefix == "ghp_" || prefix == "ghu_" || prefix == "ghs_" {
			return false
		}
	}
	if len(decoded) > 6 && decoded[:6] == "glpat-" {
		return false
	}

	// Assume encrypted if it passes all checks
	return true
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(encryptTokensCmd)
	migrateCmd.AddCommand(generateKeyCmd)

	encryptTokensCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without modifying the database")
}
