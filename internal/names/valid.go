package names

import "regexp"

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// IsValid reports whether name is 1-64 characters of letters, digits, dot,
// dash or underscore, starting with a letter or digit. Resource and
// organisation names share this rule.
func IsValid(name string) bool {
	return namePattern.MatchString(name)
}
