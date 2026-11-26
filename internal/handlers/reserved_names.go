package handlers

import "rmbl/internal/names"

// IsReservedName reports whether name, ignoring case and surrounding
// whitespace, is reserved. The list lives in internal/names so services can
// share it.
func IsReservedName(name string) bool { return names.IsReserved(name) }
