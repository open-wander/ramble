package appconfig

// ServerConfiguration Struct
type ServerConfiguration struct {
	RMBLServerHost    string
	RMBLServerPort    string
	EnableLimiter     string
	EnableLogger      string
	JWTSecret         string
	AdminEmailAddress string
}
