package handlers

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// errInvalidID is returned by paramID for a missing, non-numeric, zero or
// out-of-range id.
var errInvalidID = errors.New("invalid id")

// paramID reads a numeric id path parameter. It is the only place handlers may
// call c.Params for an id-like name (enforced by TestParamsIDOnlyInParamID):
// a request string must never reach GORM as a primary key or condition,
// because GORM treats a non-numeric string as a raw WHERE clause. Zero is
// rejected because no row has id 0.
func paramID(c *fiber.Ctx, name string) (uint, error) {
	return parseID(c.Params(name))
}

// parseID accepts only a positive base-10 integer that fits in a uint64.
func parseID(s string) (uint, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n == 0 {
		return 0, errInvalidID
	}
	return uint(n), nil
}
