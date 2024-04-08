package main

import (
	"fmt"
	"net/http"
	"testing"

	"rmbl/pkg/apptest"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"
)

// TestSetup is a test function that sets up the environment for testing.
// It prints "Setup Empty DB" and sends a GET request to the root endpoint ("/").
// It expects the response status to be http.StatusOK.
func TestSetup(t *testing.T) {
	fmt.Println("Setup Empty DB")
	e := apptest.FiberHTTPTester(t)
	e.GET("/").Expect().Status(http.StatusOK)

	fmt.Println("")
	fmt.Println("")
}

// TestSystemUserCreate is a unit test function that tests the creation of a system user.
// It performs the following steps:
// 1. Drops tables.
// 2. Initializes a Fiber HTTP tester.
// 3. Creates a system user email.
// 4. Creates a helper service.
// 5. Creates a system password using the helper service.
// 6. Constructs a system user map with the identity and password.
// 7. Logs in the system user and retrieves the token.
// 8. Retrieves the system organization.
//
// This test function does not return any values and does not throw any errors.
func TestSystemUserCreate(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	system_user_email := "test@test.int"

	helperservice, herr := helpers.NewHelperService(database.DB)
	if herr != nil {
		fmt.Println("Error Server failure")
	}

	systemPassword := helperservice.CreateSystemUser(system_user_email)

	system_user := map[string]interface{}{
		"identity": system_user_email,
		"password": systemPassword,
	}

	fmt.Println("Login System User for System User Tests")
	user_login_success := e.POST("/auth/login").WithJSON(system_user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_login_success.Keys().ContainsOnly("Status", "Message", "Data")
	user_login_success.ValueEqual("Status", "Success")
	user_login_success.ValueEqual("Message", "Success login")
	user_login_success.Value("Data").Object().Keys().ContainsOnly("Token")

	token := user_login_success.Value("Data").Object().Value("Token").String().Raw()

	fmt.Println("Get System Organisation")
	get_repos := e.GET("/org").WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	get_repos.Keys().ContainsOnly("Data", "Status", "Message", "TotalRecords")
	get_repos.ValueEqual("TotalRecords", 1)

	fmt.Println("")
	fmt.Println("")
}

// TestIndex is a unit test function that tests the behavior of the index endpoint.
// It verifies that the index endpoint returns the expected response when the database is empty,
// and when an unknown path is requested.
func TestIndex(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)
	fmt.Println("Test Empty DB Index")
	index_exist_object := e.GET("/").Expect().
		Status(http.StatusOK).
		JSON().Object()
	index_exist_object.Keys().ContainsOnly("Status", "Message", "TotalRecords", "Data")
	index_exist_object.ValueEqual("Status", "Success")
	index_exist_object.ValueEqual("Message", "No Records found")
	index_exist_object.ValueEqual("TotalRecords", 0)
	index_exist_object.ValueEqual("Data", nil)
}

// TestUserSignUp is a test function that tests the user sign-up functionality.
// It performs various test cases to validate different scenarios such as empty JSON,
// empty username, empty email, empty password, successful sign-up, and account already exists.
// The function uses the FiberHTTPTester to simulate HTTP requests and assertions to validate the responses.
// It prints the test results to the console for visibility.
func TestUserSignUp(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_empty := map[string]interface{}{
		"username":   "",
		"first_name": "",
		"last_name":  "",
		"email":      "",
		"password":   "",
	}

	user_empty_username := map[string]interface{}{
		"username":   "",
		"first_name": "",
		"last_name":  "",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	user_empty_email := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "",
		"last_name":  "",
		"email":      "",
		"password":   "P@ssw0rd1",
	}

	user_empty_password := map[string]interface{}{
		"username":   "hydrogen2",
		"first_name": "",
		"last_name":  "",
		"email":      "hydrogen2@ptable.element",
		"password":   "",
	}

	user := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "Hydro",
		"last_name":  "Gen",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	fmt.Println("Test User Signup No JSON")
	user_signup_nojson := e.POST("/auth/signup").
		Expect().
		Status(http.StatusUnsupportedMediaType).
		JSON().Object()
	user_signup_nojson.Keys().ContainsOnly("Message", "Status", "Code")
	user_signup_nojson.ValueEqual("Status", "unsupported-media-type")
	user_signup_nojson.ValueEqual("Message", "Not Valid Content Type, Expect application/json")
	user_signup_nojson.ValueEqual("Code", 415)

	fmt.Println("Test User Signup Empty JSON")
	user_signup_emptyjson := e.POST("/auth/signup").WithJSON(user_empty).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	user_signup_emptyjson.Keys().ContainsOnly("Status", "Message")
	user_signup_emptyjson.ValueEqual("Status", "validation-error")
	user_signup_emptyjson.ContainsMap(map[string]interface{}{
		"Message": map[string]interface{}{
			"email":    "validation failed on field 'email', condition: required",
			"username": "validation failed on field 'username', condition: required",
			"password": "validation failed on field 'password', condition: required",
		},
	})

	fmt.Println("Test User Signup Empty Username")
	user_signup_empty_username := e.POST("/auth/signup").WithJSON(user_empty_username).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	user_signup_empty_username.Keys().ContainsOnly("Status", "Message")
	user_signup_empty_username.ValueEqual("Status", "validation-error")
	user_signup_empty_username.ContainsMap(map[string]interface{}{
		"Message": map[string]interface{}{
			"username": "validation failed on field 'username', condition: required",
		},
	})

	fmt.Println("Test User Signup Empty Email")
	user_signup_empty_email := e.POST("/auth/signup").WithJSON(user_empty_email).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	user_signup_empty_email.Keys().ContainsOnly("Status", "Message")
	user_signup_empty_email.ValueEqual("Status", "validation-error")
	user_signup_empty_email.ContainsMap(map[string]interface{}{
		"Message": map[string]interface{}{
			"email": "validation failed on field 'email', condition: required",
		},
	})

	fmt.Println("Test User Signup Empty Password")
	user_signup_empty_password := e.POST("/auth/signup").WithJSON(user_empty_password).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	user_signup_empty_password.Keys().ContainsOnly("Status", "Message")
	user_signup_empty_password.ValueEqual("Status", "validation-error")
	user_signup_empty_password.ContainsMap(map[string]interface{}{
		"Message": map[string]interface{}{
			"password": "validation failed on field 'password', condition: required",
		},
	})

	fmt.Println("Test User Signup")
	user_signup := e.POST("/auth/signup").WithJSON(user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_signup.Keys().ContainsOnly("Status", "Message", "Data")
	user_signup.ValueEqual("Status", "Success")
	user_signup.ValueEqual("Message", "Created User")
	user_signup.ContainsMap(map[string]interface{}{
		"Data": map[string]interface{}{
			"username": "hydrogen",
			"email":    "hydrogen@ptable.element",
		},
	})

	fmt.Println("Test User SignupAccount Exists")
	user_signup_account_exists := e.POST("/auth/signup").WithJSON(user).
		Expect().
		Status(http.StatusConflict).
		JSON().Object()
	user_signup_account_exists.Keys().ContainsOnly("Status", "Message", "Data")
	user_signup_account_exists.ValueEqual("Status", "Error")
	user_signup_account_exists.ValueEqual("Message", "Couldn't create user")
	user_signup_account_exists.ValueEqual("Data", "Username or Email Already Exists")

	fmt.Println("")
	fmt.Println("")
}

// TestUserLogin is a test function that tests the user login functionality.
// It performs various login scenarios and checks the expected responses.
// This function uses the FiberHTTPTester to simulate HTTP requests and assertions.
// The test cases include scenarios such as empty JSON, empty email, empty password,
// non-standard email, and successful login.
func TestUserLogin(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_login := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "Hydro",
		"last_name":  "Gen",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	user_empty := map[string]interface{}{
		"identity": "",
		"password": "",
	}

	user_empty_email := map[string]interface{}{
		"identity": "",
		"password": "P@ssw0rd1",
	}

	user_empty_password := map[string]interface{}{
		"identity": "hydrogen@ptable.element",
		"password": "",
	}

	user_non_standard_email := map[string]interface{}{
		"identity": "hydrogen@ptableelement",
		"password": "P@ssw0rd1",
	}

	user := map[string]interface{}{
		"identity": "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	fmt.Println("Create User for Login tests")
	user_signup := e.POST("/auth/signup").WithJSON(user_for_login).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_signup.Keys().ContainsOnly("Status", "Message", "Data")
	user_signup.ValueEqual("Status", "Success")
	user_signup.ValueEqual("Message", "Created User")
	user_signup.ContainsMap(map[string]interface{}{
		"Data": map[string]interface{}{
			"username": "hydrogen",
			"email":    "hydrogen@ptable.element",
		},
	})

	fmt.Println("Test User Login No JSON")
	user_login_nojson := e.POST("/auth/login").
		Expect().
		Status(http.StatusUnsupportedMediaType).
		JSON().Object()
	user_login_nojson.Keys().ContainsOnly("Message", "Status", "Code")
	user_login_nojson.ValueEqual("Status", "unsupported-media-type")
	user_login_nojson.ValueEqual("Message", "Not Valid Content Type, Expect application/json")
	user_login_nojson.ValueEqual("Code", 415)

	fmt.Println("Test User Login Empty JSON")
	user_login_emptyjson := e.POST("/auth/login").WithJSON(user_empty).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	user_login_emptyjson.Keys().ContainsOnly("Status", "Message")
	user_login_emptyjson.ValueEqual("Status", "validation-error")
	user_login_emptyjson.ContainsMap(map[string]interface{}{
		"Message": map[string]interface{}{
			"identity": "validation failed on field 'identity', condition: required",
		},
	})

	fmt.Println("Test User Login Empty Email")
	user_login_empty_email := e.POST("/auth/login").WithJSON(user_empty_email).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	user_login_empty_email.Keys().ContainsOnly("Status", "Message")
	user_login_empty_email.ValueEqual("Status", "validation-error")
	user_login_empty_email.ContainsMap(map[string]interface{}{
		"Message": map[string]interface{}{
			"identity": "validation failed on field 'identity', condition: required",
		},
	})

	fmt.Println("Test User Login Empty Password")
	user_login_empty_password := e.POST("/auth/login").WithJSON(user_empty_password).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	user_login_empty_password.Keys().ContainsOnly("Status", "Message")
	user_login_empty_password.ValueEqual("Status", "validation-error")
	user_login_empty_password.ContainsMap(map[string]interface{}{
		"Message": map[string]interface{}{
			"password": "validation failed on field 'password', condition: required",
		},
	})

	fmt.Println("Test User Login Non Standard Email")
	user_login_non_standard_email := e.POST("/auth/login").WithJSON(user_non_standard_email).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	user_login_non_standard_email.Keys().ContainsOnly("Status", "Message")
	user_login_non_standard_email.ValueEqual("Status", "validation-error")
	user_login_non_standard_email.ContainsMap(map[string]interface{}{
		"Message": map[string]interface{}{
			"identity": "validation failed on field 'identity', condition: email, actual: hydrogen@ptableelement",
		},
	})

	fmt.Println("Test User Login Success")
	user_login_success := e.POST("/auth/login").WithJSON(user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_login_success.Keys().ContainsOnly("Status", "Message", "Data")
	user_login_success.ValueEqual("Status", "Success")
	user_login_success.ValueEqual("Message", "Success login")
	user_login_success.Value("Data").Object().Keys().ContainsOnly("Token")

	fmt.Println("")
	fmt.Println("")
}

// TestUserDeleteSelf tests the functionality of deleting a user's own account.
// It performs the following steps:
// 1. Drops the necessary tables.
// 2. Sets up the necessary test environment.
// 3. Creates a user for testing purposes.
// 4. Logs in the user.
// 5. Attempts to delete the user without logging in, and verifies the response.
// 6. Attempts to delete the user without providing a password, and verifies the response.
// 7. Deletes the user with the provided password, and verifies the response.
func TestUserDeleteSelf(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_login := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "Hydro",
		"last_name":  "Gen",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	user_password := map[string]interface{}{
		"password": "P@ssw0rd1",
	}

	user := map[string]interface{}{
		"identity": "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	// Create User for User Delete tests
	user_signup := e.POST("/auth/signup").WithJSON(user_for_login).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_id_for_delete_test := user_signup.Value("Data").Object().Value("id").String().Raw()

	// User Login for User Delet Tests
	user_login_success := e.POST("/auth/login").WithJSON(user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_token := user_login_success.Value("Data").Object().Value("Token").String().Raw()

	fmt.Println("User Tries to Delete Themself Without logging in")
	delete_user_with_no_login := e.DELETE("/user/" + user_id_for_delete_test).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	delete_user_with_no_login.Keys().ContainsOnly("Status", "Message", "Data")
	delete_user_with_no_login.ValueEqual("Status", "Error")
	delete_user_with_no_login.ValueEqual("Message", "Missing or malformed JWT")
	delete_user_with_no_login.ValueEqual("Data", nil)

	fmt.Println("User Tries to Delete Themself With No Password")
	delete_user_with_no_password := e.DELETE("/user/"+user_id_for_delete_test).WithHeader("Authorization", "Bearer "+user_token).
		Expect().
		Status(http.StatusInternalServerError).
		JSON().Object()
	delete_user_with_no_password.Keys().ContainsOnly("Status", "Message", "Data")
	delete_user_with_no_password.ValueEqual("Status", "Error")
	delete_user_with_no_password.ValueEqual("Message", "Review your input")
	delete_user_with_no_password.ValueEqual("Data", nil)

	fmt.Println("User Deletes Themself With Password")
	delete_user_with_login := e.DELETE("/user/"+user_id_for_delete_test).WithHeader("Authorization", "Bearer "+user_token).WithJSON(user_password).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	delete_user_with_login.Keys().ContainsOnly("Status", "Message", "Data")
	delete_user_with_login.ValueEqual("Status", "Success")
	delete_user_with_login.ValueEqual("Message", "User successfully deleted")
	delete_user_with_login.ValueEqual("Data", nil)

	fmt.Println("")
	fmt.Println("")
}

// TestRepoCreate is a test function that tests the creation of a repository.
// It performs the following steps:
// 1. Drops tables.
// 2. Creates a user for creating repositories.
// 3. Tries to create a repository without login
// 4. Tries to create a repository without JSON
// 5. Tries to create a repository with empty JSON
// 6. Tries to create a repository with valid JSON
func TestRepoCreate(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_create := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "Hydro",
		"last_name":  "Gen",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	user := map[string]interface{}{
		"identity": "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	repo_empty_json := map[string]interface{}{
		"name":        "",
		"description": "",
		"version":     "",
		"url":         "",
	}

	repo := map[string]interface{}{
		"name":        "testrepo",
		"description": "some cool description",
		"version":     "v1.0.0",
		"url":         "https://github.com/hydrogen/testrepo",
	}

	fmt.Println("Create User for Create Repo tests")
	user_signup := e.POST("/auth/signup").WithJSON(user_for_create).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_signup.Keys().ContainsOnly("Status", "Message", "Data")
	user_signup.ValueEqual("Status", "Success")
	user_signup.ValueEqual("Message", "Created User")
	user_signup.ContainsMap(map[string]interface{}{
		"Data": map[string]interface{}{
			"username": "hydrogen",
			"email":    "hydrogen@ptable.element",
		},
	})

	fmt.Println("Login User for Create Repo Tests")
	user_login_success := e.POST("/auth/login").WithJSON(user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_login_success.Keys().ContainsOnly("Status", "Message", "Data")
	user_login_success.ValueEqual("Status", "Success")
	user_login_success.ValueEqual("Message", "Success login")
	user_login_success.Value("Data").Object().Keys().ContainsOnly("Token")

	token := user_login_success.Value("Data").Object().Value("Token").String().Raw()

	fmt.Println("Create Repo Without Login")
	create_repo_without_login := e.POST("/hydrogen").WithJSON(repo).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	create_repo_without_login.Keys().ContainsOnly("Status", "Message", "Data")
	create_repo_without_login.ValueEqual("Status", "Error")
	create_repo_without_login.ValueEqual("Message", "Missing or malformed JWT")
	create_repo_without_login.ValueEqual("Data", nil)

	fmt.Println("Create Repo With No JSON")
	create_repo_with_no_json := e.POST("/hydrogen").WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusUnsupportedMediaType).
		JSON().Object()
	create_repo_with_no_json.Keys().ContainsOnly("Status", "Message", "Code")
	create_repo_with_no_json.ValueEqual("Status", "unsupported-media-type")
	create_repo_with_no_json.ValueEqual("Message", "Not Valid Content Type, Expect application/json")
	create_repo_with_no_json.ValueEqual("Code", 415)

	fmt.Println("Create Repo With Empty JSON")
	create_repo_with_empty_json := e.POST("/hydrogen").WithHeader("Authorization", "Bearer "+token).WithJSON(repo_empty_json).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	create_repo_with_empty_json.Keys().ContainsOnly("Status", "Message")
	create_repo_with_empty_json.ValueEqual("Status", "validation-error")
	create_repo_with_empty_json.ContainsMap(map[string]interface{}{
		"Message": map[string]interface{}{
			"description": "validation failed on field 'description', condition: required",
			"name":        "validation failed on field 'name', condition: required",
			"url":         "validation failed on field 'url', condition: required",
			"version":     "validation failed on field 'version', condition: required",
		},
	})

	fmt.Println("Create Repo With Login")
	create_repo_with_login := e.POST("/hydrogen").WithHeader("Authorization", "Bearer "+token).WithJSON(repo).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	create_repo_with_login.Keys().ContainsOnly("name", "version", "description", "url", "orgid", "id")
	create_repo_with_login.ValueEqual("name", "testrepo")
	create_repo_with_login.ValueEqual("description", "some cool description")
	create_repo_with_login.ValueEqual("version", "v1.0.0")
	create_repo_with_login.ValueEqual("url", "https://github.com/hydrogen/testrepo")
	create_repo_with_login.ContainsKey("id")
	create_repo_with_login.ContainsKey("orgid")

	fmt.Println("")
	fmt.Println("")
}

// TestRepoRead is a test function that tests the functionality of reading repositories.
// It performs the following steps:
// 1. Drops tables.
// 2. Creates a user for reading repositories.
// 3. Logs in the user.
// 4. Creates multiple repositories with the logged-in user.
// 5. Retrieves all repositories.
// 6. Retrieves the details of a specific repository.
// 7. Retrieves all repositories for a specific user.
func TestRepoRead(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_read := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "Hydro",
		"last_name":  "Gen",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	user := map[string]interface{}{
		"identity": "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	fmt.Println("Create User for Read Repo tests")
	user_signup := e.POST("/auth/signup").WithJSON(user_for_read).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_signup.Keys().ContainsOnly("Status", "Message", "Data")
	user_signup.ValueEqual("Status", "Success")
	user_signup.ValueEqual("Message", "Created User")
	user_signup.ContainsMap(map[string]interface{}{
		"Data": map[string]interface{}{
			"username": "hydrogen",
			"email":    "hydrogen@ptable.element",
		},
	})

	fmt.Println("Login User for Read Repo Tests")
	user_login_success := e.POST("/auth/login").WithJSON(user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_login_success.Keys().ContainsOnly("Status", "Message", "Data")
	user_login_success.ValueEqual("Status", "Success")
	user_login_success.ValueEqual("Message", "Success login")
	user_login_success.Value("Data").Object().Keys().ContainsOnly("Token")

	token := user_login_success.Value("Data").Object().Value("Token").String().Raw()

	fmt.Println("Create Repos With Login")
	for i := 0; i < 30; i++ {
		repo := map[string]interface{}{
			"name":        "testrepo" + fmt.Sprint(i),
			"description": "some cool description" + fmt.Sprint(i),
			"version":     "v1.0." + fmt.Sprint(i),
			"url":         "https://github.com/hydrogen/testrepo" + fmt.Sprint(i),
		}
		create_repos_with_login := e.POST("/hydrogen").WithHeader("Authorization", "Bearer "+token).WithJSON(repo).
			Expect().
			Status(http.StatusOK).
			JSON().Object()
		create_repos_with_login.Keys().ContainsOnly("name", "version", "description", "url", "orgid", "id")
	}

	fmt.Println("Get All Repositories")
	get_repos := e.GET("/").
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	get_repos.Keys().ContainsOnly("Data", "Status", "Message", "TotalRecords")
	get_repos.ValueEqual("TotalRecords", 30)

	fmt.Println("Get Repository Detail For hydrogen/testrepo2")
	get_repo_detail_repo := e.GET("/hydrogen/testrepo2").
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	get_repo_detail_repo.Keys().ContainsOnly("Data", "Status", "Message")
	get_repo_detail_repo.ValueEqual("Status", "Success")
	get_repo_detail_repo.ValueEqual("Message", "Repository Found")
	get_repo_detail_repo.ContainsMap(map[string]interface{}{
		"Data": map[string]interface{}{
			"description": "some cool description2",
			"name":        "testrepo2",
			"orgname":     "hydrogen",
			"url":         "https://github.com/hydrogen/testrepo2",
			"version":     "v1.0.2",
		},
	})

	// TODO test redirect for Nomad

	fmt.Println("Get All Repositories For hydrogen")
	get_repos_test_repo := e.GET("/hydrogen").
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	get_repos_test_repo.Keys().ContainsOnly("Data", "Status", "Message", "TotalRecords")
	get_repos_test_repo.ValueEqual("TotalRecords", 30)

	fmt.Println("")
	fmt.Println("")
}

// TestRepoUpdate is a test function that tests the update functionality of a repository.
// It performs the following steps:
// 1. Drops tables.
// 2. Creates a user for updating the repository.
// 3. Logs in the user.
// 4. Creates a repository with the logged-in user.
// 5. Attempts to update the repository without logging in (which should fail).
// 6. Updates the repository with the logged-in user.
func TestRepoUpdate(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_update := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "Hydro",
		"last_name":  "Gen",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	user := map[string]interface{}{
		"identity": "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	repo := map[string]interface{}{
		"name":        "testrepo",
		"description": "some cool description",
		"version":     "v1.0.0",
		"url":         "https://github.com/hydrogen/testrepo",
	}
	update_repo := map[string]interface{}{
		"name":        "testrepo",
		"description": "some cool description",
		"version":     "v1.0.1",
		"url":         "https://github.com/hydrogen/testrepo",
	}

	fmt.Println("Create User for Update Repo tests")
	user_signup := e.POST("/auth/signup").WithJSON(user_for_update).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_signup.Keys().ContainsOnly("Status", "Message", "Data")
	user_signup.ValueEqual("Status", "Success")
	user_signup.ValueEqual("Message", "Created User")
	user_signup.ContainsMap(map[string]interface{}{
		"Data": map[string]interface{}{
			"username": "hydrogen",
			"email":    "hydrogen@ptable.element",
		},
	})

	fmt.Println("Login User for Update Repo Tests")
	user_login_success := e.POST("/auth/login").WithJSON(user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_login_success.Keys().ContainsOnly("Status", "Message", "Data")
	user_login_success.ValueEqual("Status", "Success")
	user_login_success.ValueEqual("Message", "Success login")
	user_login_success.Value("Data").Object().Keys().ContainsOnly("Token")

	token := user_login_success.Value("Data").Object().Value("Token").String().Raw()

	fmt.Println("Create Repo With Login")
	create_repo_with_login := e.POST("/hydrogen").WithHeader("Authorization", "Bearer "+token).WithJSON(repo).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	create_repo_with_login.Keys().ContainsOnly("name", "version", "description", "url", "orgid", "id")
	create_repo_with_login.ValueEqual("name", "testrepo")
	create_repo_with_login.ValueEqual("description", "some cool description")
	create_repo_with_login.ValueEqual("version", "v1.0.0")
	create_repo_with_login.ValueEqual("url", "https://github.com/hydrogen/testrepo")
	create_repo_with_login.ContainsKey("id")
	create_repo_with_login.ContainsKey("orgid")

	fmt.Println("Update Repo Without Login")
	update_repo_without_login := e.PUT("/hydrogen/testrepo").WithJSON(update_repo).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	update_repo_without_login.Keys().ContainsOnly("Status", "Message", "Data")
	update_repo_without_login.ValueEqual("Status", "Error")
	update_repo_without_login.ValueEqual("Message", "Missing or malformed JWT")
	update_repo_without_login.ValueEqual("Data", nil)

	fmt.Println("Update Repo With Login")
	update_repo_with_login := e.PUT("/hydrogen/testrepo").WithHeader("Authorization", "Bearer "+token).WithJSON(update_repo).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	update_repo_with_login.Keys().ContainsOnly("name", "version", "description", "url", "orgid", "id")
	update_repo_with_login.ValueEqual("name", "testrepo")
	update_repo_with_login.ValueEqual("description", "some cool description")
	update_repo_with_login.ValueEqual("version", "v1.0.1")
	update_repo_with_login.ValueEqual("url", "https://github.com/hydrogen/testrepo")
	update_repo_with_login.ContainsKey("id")
	update_repo_with_login.ContainsKey("orgid")

	fmt.Println("")
	fmt.Println("")
}

// TestRepoDelete is a test function that tests the deletion of a repository.
// It performs the following steps:
// 1. Drops tables.
// 2. Creates a user for deletion.
// 3. Logs in the user.
// 4. Creates a repository.
// 5. Attempts to delete the repository without login (which should fail).
// 6. Deletes the repository with login.
func TestRepoDelete(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_delete := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "Hydro",
		"last_name":  "Gen",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	user := map[string]interface{}{
		"identity": "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	repo := map[string]interface{}{
		"name":        "testrepo",
		"description": "some cool description",
		"version":     "v1.0.0",
		"url":         "https://github.com/hydrogen/testrepo",
	}
	update_repo := map[string]interface{}{
		"name":        "testrepo",
		"description": "some cool description",
		"version":     "v1.0.1",
		"url":         "https://github.com/hydrogen/testrepo",
	}

	fmt.Println("Create User for Delete Repo tests")
	user_signup := e.POST("/auth/signup").WithJSON(user_for_delete).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_signup.Keys().ContainsOnly("Status", "Message", "Data")
	user_signup.ValueEqual("Status", "Success")
	user_signup.ValueEqual("Message", "Created User")
	user_signup.ContainsMap(map[string]interface{}{
		"Data": map[string]interface{}{
			"username": "hydrogen",
			"email":    "hydrogen@ptable.element",
		},
	})

	fmt.Println("Login User for Delete Repo Tests")
	user_login_success := e.POST("/auth/login").WithJSON(user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_login_success.Keys().ContainsOnly("Status", "Message", "Data")
	user_login_success.ValueEqual("Status", "Success")
	user_login_success.ValueEqual("Message", "Success login")
	user_login_success.Value("Data").Object().Keys().ContainsOnly("Token")

	token := user_login_success.Value("Data").Object().Value("Token").String().Raw()

	fmt.Println("Create Repo For Delete Test")
	create_repo_with_login := e.POST("/hydrogen").WithHeader("Authorization", "Bearer "+token).WithJSON(repo).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	create_repo_with_login.Keys().ContainsOnly("name", "version", "description", "url", "orgid", "id")
	create_repo_with_login.ValueEqual("name", "testrepo")
	create_repo_with_login.ValueEqual("description", "some cool description")
	create_repo_with_login.ValueEqual("version", "v1.0.0")
	create_repo_with_login.ValueEqual("url", "https://github.com/hydrogen/testrepo")
	create_repo_with_login.ContainsKey("id")
	create_repo_with_login.ContainsKey("orgid")

	fmt.Println("Delete Repo Without Login")
	delete_repo_without_login := e.DELETE("/hydrogen/testrepo").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	delete_repo_without_login.Keys().ContainsOnly("Status", "Message", "Data")
	delete_repo_without_login.ValueEqual("Status", "Error")
	delete_repo_without_login.ValueEqual("Message", "Missing or malformed JWT")
	delete_repo_without_login.ValueEqual("Data", nil)

	fmt.Println("Delete Repo With Login")
	delete_repo_with_login := e.DELETE("/hydrogen/testrepo").WithHeader("Authorization", "Bearer "+token).WithJSON(update_repo).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	delete_repo_with_login.Keys().ContainsOnly("Status", "Message", "Data")
	delete_repo_with_login.ValueEqual("Status", "Success")
	delete_repo_with_login.ValueEqual("Message", "testrepo Deleted")
	delete_repo_with_login.ValueEqual("Data", nil)

	fmt.Println("")
	fmt.Println("")
}

// TestRepoSearch is a test function that tests searching for a repository.
// It performs the following steps:
// 1. Drops tables.
// 2. Creates a user for search.
// 3. Logs in the user.
// 4. Creates 30 repositories.
// 5. Attempts to find the repository testrepo20.
// 6. Attempts to find all the repositories with testrepo2.
func TestRepoSearch(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_search := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "Hydro",
		"last_name":  "Gen",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	user := map[string]interface{}{
		"identity": "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	fmt.Println("Create User for Search Repo tests")
	user_signup := e.POST("/auth/signup").WithJSON(user_for_search).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_signup.Keys().ContainsOnly("Status", "Message", "Data")
	user_signup.ValueEqual("Status", "Success")
	user_signup.ValueEqual("Message", "Created User")
	user_signup.ContainsMap(map[string]interface{}{
		"Data": map[string]interface{}{
			"username": "hydrogen",
			"email":    "hydrogen@ptable.element",
		},
	})

	fmt.Println("Login User for Search Repo Tests")
	user_login_success := e.POST("/auth/login").WithJSON(user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_login_success.Keys().ContainsOnly("Status", "Message", "Data")
	user_login_success.ValueEqual("Status", "Success")
	user_login_success.ValueEqual("Message", "Success login")
	user_login_success.Value("Data").Object().Keys().ContainsOnly("Token")

	token := user_login_success.Value("Data").Object().Value("Token").String().Raw()

	fmt.Println("Create Repos With Login")
	for i := 0; i < 30; i++ {
		repo := map[string]interface{}{
			"name":        "testrepo" + fmt.Sprint(i),
			"description": "some cool description" + fmt.Sprint(i),
			"version":     "v1.0." + fmt.Sprint(i),
			"url":         "https://github.com/hydrogen/testrepo" + fmt.Sprint(i),
		}
		create_repos_with_login := e.POST("/hydrogen").WithHeader("Authorization", "Bearer "+token).WithJSON(repo).
			Expect().
			Status(http.StatusOK).
			JSON().Object()
		create_repos_with_login.Keys().ContainsOnly("name", "version", "description", "url", "orgid", "id")
	}

	fmt.Println("Get All Repositories that contains testrepo20")
	search_repos_single := e.GET("/").
		WithQuery("search", "testrepo20").
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	search_repos_single.Keys().ContainsOnly("Data", "Status", "Message", "TotalRecords")
	search_repos_single.ValueEqual("Status", "Success")
	search_repos_single.ValueEqual("Message", "Records found")
	search_repos_single.ValueEqual("TotalRecords", 1)

	fmt.Println("Get All Repositories that contains testrepo2")
	search_repos_multiple := e.GET("/").
		WithQuery("search", "testrepo2").
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	search_repos_multiple.Keys().ContainsOnly("Data", "Status", "Message", "TotalRecords")
	search_repos_multiple.ValueEqual("Status", "Success")
	search_repos_multiple.ValueEqual("Message", "Records found")
	search_repos_multiple.ValueEqual("TotalRecords", 11)

	fmt.Println("")
	fmt.Println("")
}

// TestSystemUserOrgs is a test function that tests SystemUser Access.
// It performs the following steps:
// 1. Drops tables.
// 2. Creates a system user.
// 3. Logs in the system user.
// 4. Creates a standard user for testing results.
// 5. Attempts to get all organisations without auth which should fail.
// 6. Attempts to get all organisations with auth.
// 7. Attempts to get all users without auth which should fail.
// 8. Attempts to get all users with auth.
func TestSystemUserOrgs(t *testing.T) {
	fmt.Println("System User Access to Organisations Test")
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	system_user_email := "test@test.int"

	helperservice, herr := helpers.NewHelperService(database.DB)
	if herr != nil {
		fmt.Println("Error Server failure")
	}

	systemPassword := helperservice.CreateSystemUser(system_user_email)

	system_user := map[string]interface{}{
		"identity": system_user_email,
		"password": systemPassword,
	}

	user_for_org_create := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "Hydro",
		"last_name":  "Gen",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	// Login System User to test organisation access
	system_user_login_success := e.POST("/auth/login").WithJSON(system_user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	system_user_token := system_user_login_success.Value("Data").Object().Value("Token").String().Raw()
	fmt.Println("Generated Token for System User")
	fmt.Println(system_user_token)

	// Create User to generate additional organisation for testing
	e.POST("/auth/signup").WithJSON(user_for_org_create).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	fmt.Println("Get All Organisations Without Auth")
	get_orgs_noauth := e.GET("/org").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	get_orgs_noauth.Keys().ContainsOnly("Status", "Message", "Data")
	get_orgs_noauth.ValueEqual("Status", "Error")
	get_orgs_noauth.ValueEqual("Message", "Missing or malformed JWT")
	get_orgs_noauth.ValueEqual("Data", nil)

	fmt.Println("Get All Organisations With Auth")
	get_orgs_auth := e.GET("/org").WithHeader("Authorization", "Bearer "+system_user_token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	get_orgs_auth.Keys().ContainsOnly("Data", "Status", "Message", "TotalRecords")
	get_orgs_auth.ValueEqual("TotalRecords", 2)

	fmt.Println("Get All Users Without Auth")
	get_users_noauth := e.GET("/user").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object()
	get_users_noauth.Keys().ContainsOnly("Status", "Message", "Data")
	get_users_noauth.ValueEqual("Status", "Error")
	get_users_noauth.ValueEqual("Message", "Missing or malformed JWT")
	get_users_noauth.ValueEqual("Data", nil)

	fmt.Println("Get All Users")
	get_users_auth := e.GET("/user").WithHeader("Authorization", "Bearer "+system_user_token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	get_users_auth.Keys().ContainsOnly("Data", "Status", "Message", "TotalRecords")
	get_users_auth.ValueEqual("TotalRecords", 2)

	fmt.Println("")
	fmt.Println("")
}

// TestSystemUserCRUDUsers is a test function that performs CRUD operations on system users.
// It tests the creation and deletion of users using the system user account.
// The function sets up the necessary data, such as system user credentials, user details,
// and repository information, and then performs the following steps:
//  1. Creates a system user and retrieves the generated system user token.
//  2. Creates a user for organization testing and retrieves the user's ID.
//  3. Logs in as the created system user and retrieves the user's token.
//  4. Creates a repository for deletion testing.
//  5. Attempts to delete the user with the system user login but without providing a password,
//     expecting an error response.
//  6. Deletes the user with the system user login and provides the system user password,
//     expecting a success response.
func TestSystemUserCRUDUsers(t *testing.T) {
	fmt.Println("System User CRUD on Users")
	fmt.Println("")
	fmt.Println("")
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	system_user_email := "test@test.int"

	helperservice, herr := helpers.NewHelperService(database.DB)
	if herr != nil {
		fmt.Println("Error Server failure")
	}

	systemPassword := helperservice.CreateSystemUser(system_user_email)

	system_user := map[string]interface{}{
		"identity": system_user_email,
		"password": systemPassword,
	}
	system_user_password := map[string]interface{}{
		"password": systemPassword,
	}

	user_for_org_create := map[string]interface{}{
		"username":   "hydrogen",
		"first_name": "Hydro",
		"last_name":  "Gen",
		"email":      "hydrogen@ptable.element",
		"password":   "P@ssw0rd1",
	}

	user_login := map[string]interface{}{
		"identity": "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	repo := map[string]interface{}{
		"name":        "testrepo",
		"description": "some cool description",
		"version":     "v1.0.0",
		"url":         "https://github.com/hydrogen/testrepo",
	}

	// Login System User to test organisation access
	system_user_login_success := e.POST("/auth/login").WithJSON(system_user).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	system_user_token := system_user_login_success.Value("Data").Object().Value("Token").String().Raw()
	fmt.Println("Generated Token for System User")
	fmt.Println(system_user_token)
	// Create User to generate additional organisation for testing
	user_signup := e.POST("/auth/signup").WithJSON(user_for_org_create).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	user_id_for_delete_test := user_signup.Value("Data").Object().Value("id").String().Raw()

	// Login User for Delete Org Tests
	user_login_success := e.POST("/auth/login").WithJSON(user_login).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	user_token_for_delete_test := user_login_success.Value("Data").Object().Value("Token").String().Raw()

	// Create Repo For Delete Test
	e.POST("/hydrogen").WithHeader("Authorization", "Bearer "+user_token_for_delete_test).WithJSON(repo).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	fmt.Println("Delete User With System User Login No Password")
	delete_user_with_system_login_no_password := e.DELETE("/user/"+user_id_for_delete_test).WithHeader("Authorization", "Bearer "+system_user_token).
		Expect().
		Status(http.StatusInternalServerError).
		JSON().Object()
	delete_user_with_system_login_no_password.Keys().ContainsOnly("Status", "Message", "Data")
	delete_user_with_system_login_no_password.ValueEqual("Status", "Error")
	delete_user_with_system_login_no_password.ValueEqual("Message", "Review your input")
	delete_user_with_system_login_no_password.ValueEqual("Data", nil)

	// TODO - Figure out a way to make sure the admin token is valid.
	// THERE IS NO PROPER ADMIN CHECK RIGHT NOW
	fmt.Println("Delete User With System User Login")
	delete_user_with_system_login := e.DELETE("/user/"+user_id_for_delete_test).WithHeader("Authorization", "Bearer "+system_user_token).WithJSON(system_user_password).
		Expect().
		Status(http.StatusOK).
		JSON().Object()
	delete_user_with_system_login.Keys().ContainsOnly("Status", "Message", "Data")
	delete_user_with_system_login.ValueEqual("Status", "Success")
	delete_user_with_system_login.ValueEqual("Message", "User successfully deleted")
	delete_user_with_system_login.ValueEqual("Data", nil)

	fmt.Println("")
	fmt.Println("")
}
