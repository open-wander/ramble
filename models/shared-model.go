package models

// Org Data is generated for filtering and pagination of Organizations
type OrgData struct {
	Status       string
	Message      string
	TotalRecords int64
	Data         []Organization
}

// // Repo Data is generated for filtering and pagination of Repositories
// type RepoData struct {
// 	TotalData    int64
// 	FilteredData int64
// 	Data         []RepositoryViewStruct
// 	// Data []Organization
// }

// Repo Data is generated for filtering and pagination of Repositories
type RepoData struct {
	Status       string
	Message      string
	TotalRecords int64
	Data         []Repository
	// Data []Organization
}

// User Data is generated for filtering and pagination of Users
type UserData struct {
	Status       string
	Message      string
	TotalRecords int64
	Data         []User
}
