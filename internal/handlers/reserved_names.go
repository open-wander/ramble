package handlers

import "strings"

// reservedNames are names that may not be used as a username or organisation
// name, because /:username would shadow or impersonate a top-level route.
// All entries are lowercase; Fiber routing is case-insensitive. The router
// test in internal/server fails when a top-level route segment is missing here.
var reservedNames = map[string]struct{}{
	// Top-level route segments registered in registerRoutes.
	"public": {}, "favicon.ico": {}, "robots.txt": {}, ".well-known": {},
	"sitemap.xml": {}, "swagger": {}, "api-docs": {}, "search": {},
	"packs": {}, "jobs": {}, "registries": {}, "docs": {}, "about": {},
	"requests": {}, "login": {}, "signup": {}, "logout": {},
	"forgot-password": {}, "reset-password": {}, "verify-email": {},
	"resend-verification": {}, "orgs": {}, "auth": {}, "admin": {},
	"new": {}, "resource": {}, "api": {}, "v1": {}, "webhooks": {},

	// Generic names kept for future routes and impersonation.
	"static": {}, "assets": {}, "health": {}, "metrics": {}, "www": {},
	"root": {}, "support": {}, "security": {}, "settings": {}, "help": {},
	"user": {}, "users": {}, "org": {}, "organizations": {}, "explore": {},
}

// IsReservedName reports whether name, ignoring case and surrounding
// whitespace, is reserved.
func IsReservedName(name string) bool {
	_, ok := reservedNames[strings.ToLower(strings.TrimSpace(name))]
	return ok
}
