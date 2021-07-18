package apptest

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
func FiberHTTPTester(t *testing.T) *httpexpect.Expect {
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
