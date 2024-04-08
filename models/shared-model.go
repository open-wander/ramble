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

// Single Repo Data is generated for returning a single repository detail
type SingleRepoData struct {
	Status       string
	Message      string
	TotalRecords int64
	Data         RepositoryViewStruct
}

// User Data is generated for filtering and pagination of Users
type UserData struct {
	Status       string
	Message      string
	TotalRecords int64
	Data         []User
}

// SingleUserData is generated for returning a single user detail
type SingleUserData struct {
	Status       string
	Message      string
	TotalRecords int64
	Data         User
}

type JWTToken struct {
	Token string
}
