package main

import (
	"net/http"
	"rmbl/api"
	"rmbl/pkg/server"
	"testing"

	"github.com/gavv/httpexpect/v2"
	"github.com/gofiber/fiber/v2"
)

func setupTestApp() *fiber.App {
	app := server.Create()
	api.Setup(app)

	return app
}

//fiberHTTPTester returns a new Expect instance to test fiberHandler().
func fiberHTTPTester(t *testing.T) *httpexpect.Expect {
	app := setupTestApp()
	return httpexpect.WithConfig(httpexpect.Config{
		// Pass requests directly to FastHTTPHandler.
		Client: &http.Client{
			Transport: httpexpect.NewFastBinder(app.Handler()),
			Jar:       httpexpect.NewJar(),
		},
		// Report errors using testify.
		Reporter: httpexpect.NewAssertReporter(t),
	})
}

func TestIndex(t *testing.T) {
	e := fiberHTTPTester(t)

	index_exist_object := e.GET("/").Expect().
		Status(http.StatusOK).
		JSON().Object()
	index_exist_object.Keys().ContainsOnly("Status", "Message", "TotalRecords", "Data")
	index_exist_object.ValueEqual("Status", "Success")
	index_exist_object.ValueEqual("Message", "No Records found")
	index_exist_object.ValueEqual("TotalRecords", 0)
	index_exist_object.ValueEqual("Data", nil)

	index_not_exist_object := e.GET("/i-dont-exist").Expect().
		Status(http.StatusOK).
		JSON().Object()
	index_not_exist_object.Keys().ContainsOnly("Status", "Message", "TotalRecords", "Data")
	index_not_exist_object.ValueEqual("Status", "Success")
	index_not_exist_object.ValueEqual("Message", "No Records found")
	index_not_exist_object.ValueEqual("TotalRecords", 0)
	index_not_exist_object.ValueEqual("Data", nil)
}
