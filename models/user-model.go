package models

import "rmbl/pkg/database"

// User struct
type User struct {
	database.DefaultModel
	Username     string       `json:"username"`
	Email        string       `json:"email"`
	Password     string       `json:"password"`
	Names        string       `json:"names"`
	Repositories []Repository `json:"repositories"`
}
