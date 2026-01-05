package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"rmbl/internal/database"
	"rmbl/internal/handlers"
	"rmbl/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	_ "rmbl/docs"

	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/swagger"
	"github.com/gofiber/template/html/v2"
)

// @title RMBL Nomad Registry API
// @version 1.0
// @description This is the API documentation for the RMBL Nomad Job & Pack Registry.
// @contact.name API Support
// @license.name MPL-2.0
// @host localhost:3000
// @BasePath /
func main() {
	seedFlag := flag.Bool("seed", false, "Seed initial user from environment variables")
	flag.Parse()

	// 1. Connect to Database
	database.Connect()
	handlers.InitSession()

	if *seedFlag || os.Getenv("AUTO_SEED") == "true" {
		database.SeedInitialUser(database.DB)
	}

	// 2. Setup Template Engine
	engine := html.New("./views", ".html")
	if os.Getenv("ENV") != "production" {
		engine.Reload(true) // Reload templates on each render (development mode only)
	}
	engine.AddFunc("dict", func(values ...interface{}) (map[string]interface{}, error) {
		if len(values)%2 != 0 {
			return nil, fmt.Errorf("invalid dict call")
		}
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	})
	engine.AddFunc("add", func(a, b int) int {
		return a + b
	})
	engine.AddFunc("upper", func(s string) string {
		return strings.ToUpper(s)
	})
	engine.AddFunc("capitalize", func(s string) string {
		if len(s) == 0 {
			return ""
		}
		return strings.ToUpper(s[:1]) + s[1:]
	})

	// 3. Setup Fiber
	app := fiber.New(fiber.Config{
		Views: engine,
	})

	// 4. Middleware
	app.Use(logger.New())

	// HTTPS enforcement middleware (only in production)
	app.Use(func(c *fiber.Ctx) error {
		if os.Getenv("ENV") == "production" && c.Protocol() != "https" {
			return c.Redirect("https://"+c.Hostname()+c.OriginalURL(), fiber.StatusMovedPermanently)
		}
		return c.Next()
	})

	app.Use(helmet.New(helmet.Config{
		// CSP: Removed 'unsafe-eval', kept 'unsafe-inline' temporarily for HTMX compatibility
		// TODO: Replace 'unsafe-inline' with nonces or move inline scripts to external files
		ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline' https://unpkg.com https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data: https://github.com https://avatars.githubusercontent.com; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; upgrade-insecure-requests;",
		CrossOriginEmbedderPolicy: "unsafe-none",
		CrossOriginResourcePolicy: "cross-origin",
		XFrameOptions:             "DENY",
		ContentTypeNosniff:        "nosniff",
		XSSProtection:             "1; mode=block",
	}))

	// CSRF Middleware
	app.Use(csrf.New(csrf.Config{
		KeyLookup:      "header:X-CSRF-Token",
		ContextKey:     "csrf",
		CookieName:     "csrf_token",
		CookieSameSite: "Lax",
		CookieSecure:   os.Getenv("ENV") == "production", // Only send over HTTPS in production
		CookieHTTPOnly: true,                             // Prevent XSS access
		Expiration:     1 * time.Hour,                    // Token expires after 1 hour
	}))

	// Middleware to check session and pass user to views
	app.Use(func(c *fiber.Ctx) error {
		sess, err := handlers.Store.Get(c)
		if err == nil {
			if userID := sess.Get("user_id"); userID != nil {
				var user models.User
				if err := database.DB.Preload("Memberships.Organization").First(&user, userID).Error; err == nil {
					c.Locals("UserID", userID)
					c.Locals("User", user)
				}
			}

			// Handle Flash Messages
			flashType := sess.Get("flash_type")
			flashMessage := sess.Get("flash_message")
			if flashType != nil && flashMessage != nil {
				c.Locals("Flash", handlers.Flash{
					Type:    flashType.(string),
					Message: flashMessage.(string),
				})
				// Clear flash after reading
				sess.Delete("flash_type")
				sess.Delete("flash_message")
				if err := sess.Save(); err != nil {
					log.Printf("Error saving session in middleware: %v", err)
				}
			}
		}

		// Always pass CSRF token to templates
		c.Locals("CSRFToken", c.Locals("csrf"))

		return c.Next()
	})

	// 5. Static Files
	app.Static("/public", "./public")
	app.Static("/favicon.ico", "./public/favicon.ico")

	// SEO Routes
	app.Get("/sitemap.xml", handlers.GenerateSitemap)

	// Swagger UI (disabled in production)
	if os.Getenv("ENV") != "production" {
		app.Get("/swagger/*", swagger.HandlerDefault)
	}

	// 6. Routes
	app.Get("/", handlers.Home)
		app.Get("/search", handlers.Search)
		app.Get("/packs", handlers.GetPacks)
		app.Get("/jobs", handlers.GetJobs)
		app.Get("/registries", handlers.GetRegistries)
		app.Get("/docs", handlers.GetDocs)
		app.Get("/about", handlers.GetAbout)

		// Auth Routes
		// Rate limiter for authentication endpoints (5 attempts per 15 minutes)
		authLimiter := limiter.New(limiter.Config{
			Max:        5,
			Expiration: 15 * time.Minute,
			KeyGenerator: func(c *fiber.Ctx) string {
				return c.IP() // Rate limit by IP address
			},
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(fiber.StatusTooManyRequests).SendString("Too many requests. Please try again later.")
			},
		})

	app.Get("/login", handlers.GetLogin)
	app.Post("/login", authLimiter, handlers.PostLogin)
	app.Get("/signup", handlers.GetSignup)
	app.Post("/signup", authLimiter, handlers.PostSignup)
	app.Get("/logout", handlers.Logout)
	app.Get("/forgot-password", handlers.GetForgotPassword)
	app.Post("/forgot-password", authLimiter, handlers.PostForgotPassword)
	app.Get("/reset-password", handlers.GetResetPassword)
	app.Post("/reset-password", authLimiter, handlers.PostResetPassword)
	app.Get("/verify-email", handlers.GetVerifyEmail)
	app.Post("/resend-verification", authLimiter, handlers.PostResendVerification)

	// Org Routes
	app.Get("/orgs/new", handlers.RequireAuth, handlers.GetCreateOrg)
	app.Post("/orgs/new", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostCreateOrg)
	app.Get("/orgs/:orgname/settings", handlers.RequireAuth, handlers.RequireOrgOwner, handlers.GetOrgSettings)
	app.Post("/orgs/:orgname/update", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.RequireOrgOwner, handlers.PostUpdateOrg)
	app.Post("/orgs/:orgname/members/add", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.RequireOrgOwner, handlers.PostAddMember)
	app.Post("/orgs/:orgname/members/:member_id/remove", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.RequireOrgOwner, handlers.PostRemoveMember)

	// OAuth Routes
	app.Get("/auth/:provider", handlers.BeginAuth)
	app.Get("/auth/:provider/callback", handlers.AuthCallback)

	// Admin Routes
	admin := app.Group("/admin", handlers.RequireAdmin)
	admin.Get("/", handlers.GetAdminDashboard)
	admin.Get("/users", handlers.GetAdminUsers)
	admin.Get("/resources", handlers.GetAdminResources)
	admin.Get("/organizations", handlers.GetAdminOrganizations)
	admin.Get("/users/:id/edit", handlers.GetEditUser)
	admin.Post("/users/:id/edit", handlers.PostEditUser)
	admin.Post("/users/:id/toggle-admin", handlers.PostToggleAdmin)
	admin.Delete("/users/:id", handlers.DeleteUser)
	admin.Get("/organizations/:id/edit", handlers.GetEditOrganization)
	admin.Post("/organizations/:id/edit", handlers.PostEditOrganization)
	admin.Delete("/organizations/:id", handlers.DeleteOrganization)

	// Resource Routes
	app.Get("/new", handlers.RequireAuth, handlers.GetNewResource)
	app.Get("/new/my-repos", handlers.RequireAuth, handlers.GetMyRepos)
	app.Get("/new/fetch-info", handlers.RequireAuth, handlers.FetchInfo)
	app.Post("/new", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostNewResource)
	app.Delete("/resource/:id", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.DeleteResource)
	app.Post("/resource/:id/webhook", handlers.HandleWebhook)
	app.Post("/resource/:id/webhook/reset", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostResetWebhookSecret)
	app.Get("/resource/:id/fetch-readme", handlers.FetchReadme)
	app.Get("/resource/:id/new-version", handlers.RequireAuth, handlers.GetNewVersion)
	app.Post("/resource/:id/version", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostNewVersion)
	app.Post("/resource/:id/star", handlers.RequireAuth, handlers.ToggleStar)
	app.Get("/:username/:resourcename/edit", handlers.RequireAuth, handlers.GetEditResource)
	app.Post("/resource/:id/edit", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostEditResource)

	// Dev Routes (disabled in production)
	if os.Getenv("ENV") != "production" {
		app.Get("/api/dev/packs", handlers.GetDevelopmentPacks)
	}

	// Namespaced Routes
	app.Get("/:username", handlers.GetUserProfile)
	app.Get("/:username/:resourcename", handlers.GetResource)
	app.Get("/:username/:resourcename/v", handlers.GetResourceVersion)
	app.Get("/:username/:resourcename/raw", handlers.GetRawResource)
	app.Get("/:username/:resourcename/v/:version/raw", handlers.GetRawResourceVersion)

	// Nomad Pack Registry API (Global)
	app.Get("/v1/packs", handlers.ListAllPacksAPI)
	app.Get("/v1/packs/search", handlers.SearchPacksAPI)
	app.Get("/v1/registries", handlers.ListUserRegistriesAPI)

	// Nomad Pack Registry API (Namespaced)
	app.Get("/:username/v1/packs", handlers.ListPacksAPI)
	app.Get("/:username/v1/packs/:packname", handlers.GetPackAPI)

	// 7. Start Server
	log.Fatal(app.Listen(":3000"))
}
