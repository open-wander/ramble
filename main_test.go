package main

import (
	"fmt"
	"net/http"
	"rmbl/pkg/apptest"
	"testing"
)

func TestSetup(t *testing.T) {
	fmt.Println("Setup Empty DB")
	e := apptest.FiberHTTPTester(t)
	e.GET("/").Expect().Status(http.StatusOK)
}

// Testing API / path
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

	fmt.Println("Test Index with unknown path")
	index_not_exist_object := e.GET("/i-dont-exist").Expect().
		Status(http.StatusOK).
		JSON().Object()
	index_not_exist_object.Keys().ContainsOnly("Status", "Message", "TotalRecords", "Data")
	index_not_exist_object.ValueEqual("Status", "Success")
	index_not_exist_object.ValueEqual("Message", "No Records found")
	index_not_exist_object.ValueEqual("TotalRecords", 0)
	index_not_exist_object.ValueEqual("Data", nil)

	fmt.Println("")
	fmt.Println("")
}

// Testing the /auth/signup path
func TestSignUp(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_empty := map[string]interface{}{
		"username": "",
		"email":    "",
		"password": "",
	}

	user_empty_username := map[string]interface{}{
		"username": "",
		"email":    "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	user_empty_email := map[string]interface{}{
		"username": "hydrogen",
		"email":    "",
		"password": "P@ssw0rd1",
	}

	user_empty_password := map[string]interface{}{
		"username": "hydrogen2",
		"email":    "hydrogen2@ptable.element",
		"password": "",
	}

	user := map[string]interface{}{
		"username": "hydrogen",
		"email":    "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
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

// test /auth/login path
func TestLogin(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_login := map[string]interface{}{
		"username": "hydrogen",
		"email":    "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
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

// Testing the Repository workflow
// Order CRUD

func TestRepoCreate(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_login := map[string]interface{}{
		"username": "hydrogen",
		"email":    "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
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

func TestRepoRead(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_login := map[string]interface{}{
		"username": "hydrogen",
		"email":    "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	user := map[string]interface{}{
		"identity": "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
	}

	fmt.Println("Create User for Create Repo tests")
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

func TestRepoUpdate(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_login := map[string]interface{}{
		"username": "hydrogen",
		"email":    "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
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

	fmt.Println("Create User for Create Repo tests")
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

func TestRepoDelete(t *testing.T) {
	apptest.DropTables()
	e := apptest.FiberHTTPTester(t)

	user_for_login := map[string]interface{}{
		"username": "hydrogen",
		"email":    "hydrogen@ptable.element",
		"password": "P@ssw0rd1",
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

	fmt.Println("Create User for Create Repo tests")
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
