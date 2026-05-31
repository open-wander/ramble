package security

// AttackCase represents a single webhook signature attack scenario.
type AttackCase struct {
	Name        string // Descriptive attack name
	Description string // What the attack attempts
	Signature   string // The malicious/malformed signature
	Timestamp   string // The timestamp to use
	Body        []byte // The body bytes
	ExpectValid bool   // Should always be false for attacks
}

// WebhookAttacks contains a comprehensive set of attack scenarios for webhook signature validation.
var WebhookAttacks = []AttackCase{
	{
		Name:        "MissingSignatureHeader",
		Description: "Empty signature header - should reject with no signature",
		Signature:   "",
		Timestamp:   GenerateValidTimestamp(),
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
	{
		Name:        "MissingSha256Prefix",
		Description: "Signature without sha256= prefix - malformed format",
		Signature:   "deadbeef123456789abcdef",
		Timestamp:   GenerateValidTimestamp(),
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
	{
		Name:        "TruncatedSignature",
		Description: "Signature cut short - incomplete hash",
		Signature:   "sha256=deadbe",
		Timestamp:   GenerateValidTimestamp(),
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
	{
		Name:        "WrongAlgorithmPrefix",
		Description: "Wrong algorithm prefix (sha512 instead of sha256)",
		Signature:   "sha512=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Timestamp:   GenerateValidTimestamp(),
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
	{
		Name:        "NonHexSignature",
		Description: "Invalid hex characters after prefix",
		Signature:   "sha256=notahexstring!!!",
		Timestamp:   GenerateValidTimestamp(),
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
	{
		Name:        "EmptyAfterPrefix",
		Description: "Just sha256= with no hash",
		Signature:   "sha256=",
		Timestamp:   GenerateValidTimestamp(),
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
	{
		Name:        "ModifiedBody",
		Description: "Valid signature but body tampered - signature mismatch",
		Signature:   ComputeValidSignature([]byte("original-body"), GenerateValidTimestamp(), "test-secret"),
		Timestamp:   GenerateValidTimestamp(),
		Body:        []byte("tampered-body"),
		ExpectValid: false,
	},
	{
		Name:        "ReplayExpiredTimestamp",
		Description: "Valid signature but timestamp more than 5 minutes old - replay attack",
		Signature: func() string {
			ts := GenerateExpiredTimestamp()
			return ComputeValidSignature([]byte("test-body"), ts, "test-secret")
		}(),
		Timestamp:   GenerateExpiredTimestamp(),
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
	{
		Name:        "ReplayFutureTimestamp",
		Description: "Valid signature but timestamp more than 5 minutes in future - clock skew attack",
		Signature: func() string {
			ts := GenerateFutureTimestamp()
			return ComputeValidSignature([]byte("test-body"), ts, "test-secret")
		}(),
		Timestamp:   GenerateFutureTimestamp(),
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
	{
		Name:        "InvalidTimestampFormat",
		Description: "Non-numeric timestamp - parsing failure",
		Signature:   "sha256=deadbeef",
		Timestamp:   "not-a-number",
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
	{
		Name:        "EmptyTimestamp",
		Description: "Empty timestamp header - missing required field",
		Signature:   "sha256=deadbeef",
		Timestamp:   "",
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
	{
		Name:        "SignatureCaseSensitivity",
		Description: "Signature with uppercase hex - case sensitivity test",
		Signature:   "sha256=DEADBEEF",
		Timestamp:   GenerateValidTimestamp(),
		Body:        []byte("test-body"),
		ExpectValid: false,
	},
}
