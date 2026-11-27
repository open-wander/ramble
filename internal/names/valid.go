package names

import "regexp"

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// IsValid reports whether name is 1-64 characters of letters, digits, dot,
// dash or underscore, starting with a letter or digit. Resource and
// organisation names share this rule.
func IsValid(name string) bool {
	return namePattern.MatchString(name)
}

var versionPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]{0,63}$`)

// IsValidVersion reports whether v is a safe version label: 1-64 characters of
// letters, digits, dot, dash, underscore or plus, starting with a letter or
// digit. It accepts tags such as v1.2.3, 1.0.0, v1.0.0-rc.1 and
// release-2024.01.
//
// A version is used as a URL path segment (/v/:version/raw), inside archive
// download URLs and as a single directory name in the CLI pack cache. Tags
// containing "/" are therefore rejected on purpose: the raw route cannot match
// them, the cache needs one path segment, and GitHub and GitLab archive URLs do
// not reliably accept an escaped %2F. Path separators, "..", "#", "?", "%",
// whitespace and control characters are rejected by the allowlist.
func IsValidVersion(v string) bool {
	return versionPattern.MatchString(v)
}
