package models

// Org Data is generated for filtering and pagination of Organizations
type OrgData struct {
	Status       string
	Message      string
	TotalRecords int64
	Data         []Organization
}

// Repo Data is generated for filtering and pagination of Repositories
type RepoData struct {
	Status       string
	Message      string
	TotalRecords int64
	Data         []RepositoryViewStruct
}

// User Data is generated for filtering and pagination of Users
type UserData struct {
	Status       string
	Message      string
	TotalRecords int64
	Data         []User
}
