package main

import (
	"fmt"
	"net/http"
	"rmbl/pkg/apptest"
	"testing"
)

//Testing API / path
func TestIndex(t *testing.T) {
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
	user_login_success.ValueEqual("Status", "success")
	user_login_success.ValueEqual("Message", "Success login")
	user_login_success.Value("Data").Object().Keys().ContainsOnly("Token")

	fmt.Println("")
	fmt.Println("")
}
