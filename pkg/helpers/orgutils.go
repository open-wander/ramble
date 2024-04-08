package helpers

import (
	"errors"

	"rmbl/models"
	"rmbl/pkg/apperr"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetOrganizationIDByUserName retrieves the organization ID associated with a given username.
// It queries the database to find the organization with the matching org_name and returns its ID.
func (s *HelperService) GetOrganizationIDByUserName(username string) (string uuid.UUID, error error) {
	// TODO: add error checking to this function
	// or move it to helpers
	var org models.Organization
	user, err := s.GetUserByUsername(username)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return org.ID, apperr.EntityNotFound(username)
	}
	orgerr := s.db.Where("user_id = ?", user.ID).Find(&org).Error
	if errors.Is(orgerr, gorm.ErrRecordNotFound) {
		return org.ID, apperr.EntityNotFound(username)
	}
	return org.ID, err
}

// GetOrganizationIDByOrgName retrieves the organization ID associated with a given username.
// It queries the database to find the organization with the matching org_name and returns its ID.
func (s *HelperService) GetOrganizationIDByOrgName(orgname string) (string uuid.UUID, error error) {
	// TODO: add error checking to this function
	// or move it to helpers
	var org models.Organization
	err := s.db.Where("org_name = ?", orgname).First(&org).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return org.ID, apperr.EntityNotFound(orgname)
	}
	return org.ID, err
}
