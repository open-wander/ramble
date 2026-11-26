// Package org holds organisation and membership behaviour shared by the web
// handlers and the /v1 API.
package org

import (
	"errors"
	"fmt"
)

const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

var (
	ErrNameRequired    = errors.New("org: name is required")
	ErrNameInvalid     = errors.New("org: name is invalid")
	ErrNameReserved    = errors.New("org: name is reserved")
	ErrNameTakenByUser = errors.New("org: name is taken by a user")
	ErrNameTaken       = errors.New("org: name is taken")
	ErrNotFound        = errors.New("org: not found")
	ErrNotOwner        = errors.New("org: not an owner")
	ErrNotMember       = errors.New("org: not a member")
	ErrAlreadyMember   = errors.New("org: already a member")
	ErrUserNotFound    = errors.New("org: user not found")
	ErrLastOwner       = errors.New("org: cannot remove the last owner")
	ErrInvalidRole     = errors.New("org: invalid role")
)

// NotEmptyError is returned by DeleteOrg when live resources still belong to
// the organisation.
type NotEmptyError struct{ Count int64 }

func (e *NotEmptyError) Error() string {
	return fmt.Sprintf("org: still has %d resources", e.Count)
}

// IsInputError reports whether err is a fault in the caller's input. Not
// found, not owner, not member and last owner are mapped by callers one by
// one because their status differs between web and API.
func IsInputError(err error) bool {
	for _, target := range []error{
		ErrNameRequired, ErrNameInvalid, ErrNameReserved, ErrNameTakenByUser, ErrNameTaken,
		ErrInvalidRole, ErrAlreadyMember,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
