package apptest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"rmbl/api"
	"rmbl/models"
	"rmbl/pkg/apperr"
	appconfig "rmbl/pkg/config"
	"rmbl/pkg/database"

	"github.com/gavv/httpexpect/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/etag"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/helmet/v2"
)

func setupMiddlewares(app *fiber.App) {
	appconfig := appconfig.GetConfig()
	app.Use(helmet.New())
	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed, // 1
	}))
	app.Use(etag.New())
	if appconfig.Server.EnableLimiter != "false" {
		app.Use(limiter.New(
			limiter.Config{
				Max:        20,
				Expiration: 30 * time.Second,
				LimitReached: func(c *fiber.Ctx) error {
					return apperr.RateLimit()
				},
			}))
	}
	if appconfig.Server.EnableLogger != "" {
		app.Use(logger.New())
	}
}

func create() *fiber.App {
	database.SetupTestDatabase()
	app := fiber.New(fiber.Config{
		// Override default error handler
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			if e, ok := err.(*apperr.Error); ok {
				return ctx.Status(e.Code).JSON(e)
			} else if e, ok := err.(*fiber.Error); ok {
				return ctx.Status(e.Code).JSON(apperr.Error{Status: "internal-server", Code: e.Code, Message: e.Message})
			} else {
				return ctx.Status(fiber.StatusInternalServerError).JSON(apperr.Error{Status: "internal-server", Code: 500, Message: err.Error()})
			}
		},
	})
	database.DB.AutoMigrate(&models.Organization{})
	database.DB.AutoMigrate(&models.Repository{})
	database.DB.AutoMigrate(&models.User{})

	setupMiddlewares(app)

	return app
}

// Drop Tables after each test run
func DropTables() {
	database.DB.Migrator().DropTable(&models.User{})
	database.DB.Migrator().DropTable(&models.Repository{})
	database.DB.Migrator().DropTable(&models.Organization{})
	fmt.Println("Tables Dropped")
}

func setupTestApp() *fiber.App {
	app := create()
	api.Setup(app)

	return app
}

// fiberHTTPTester returns a new Expect instance to test fiberHandler().
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
