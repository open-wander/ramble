package server

import (
	"fmt"
	"time"

	"rmbl/pkg/apperr"
	appconfig "rmbl/pkg/config"
	"rmbl/pkg/database"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/etag"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/helmet/v2"
)

// setupMiddlewares sets up the middlewares for the Fiber app.
// It takes an instance of the Fiber app as a parameter and applies various middlewares to it.
// The middlewares include helmet, recover, cors, compress, etag, limiter, and logger.
// The specific configuration for each middleware is defined in the appconfig.
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

// Create creates a new instance of the fiber.App and returns it.
// It also sets up the database and adds middlewares to the app.
func Create() *fiber.App {
	database.SetupDatabase()

	app := fiber.New(fiber.Config{
		// // Override default error handler
		// ErrorHandler: func(ctx *fiber.Ctx, err error) error {
		// 	if e, ok := err.(*apperr.Error); ok {
		// 		return ctx.Status(e.Code).JSON(e)
		// 	} else if e, ok := err.(*fiber.Error); ok {
		// 		return ctx.Status(e.Code).JSON(apperr.Error{Status: "internal-server", Code: e.Code, Message: e.Message})
		// 	} else {
		// 		return ctx.Status(fiber.StatusInternalServerError).JSON(apperr.Error{Status: "internal-server", Code: 500, Message: err.Error()})
		// 	}
		// },
	})

	setupMiddlewares(app)

	return app
}

// Listen starts the server and listens for incoming requests.
// It takes an instance of the fiber.App as a parameter.
// Returns an error if there was a problem starting the server.
// The server listens on the host and port specified in the app configuration.
// If a request is not found, a 404 status is returned.
func Listen(app *fiber.App) error {
	appconfig := appconfig.GetConfig()

	// 404 Handler
	app.Use(func(c *fiber.Ctx) error {
		return c.SendStatus(404)
	})
	fmt.Println("Rest API v0.1 - RMBL API")
	return app.Listen(fmt.Sprintf("%s:%s", appconfig.Server.RMBLServerHost, appconfig.Server.RMBLServerPort))
}
