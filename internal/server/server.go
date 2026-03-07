package server

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"rmbl/internal/database"
	"rmbl/internal/handlers"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	resourcesvc "rmbl/internal/services/resource"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/gofiber/swagger"
	"github.com/gofiber/template/html/v2"
	_ "rmbl/api-docs"
)

// Config holds server configuration options
type Config struct {
	Port    string
	Seed    bool
	Version string
}

// Run starts the Ramble web server
func Run(cfg Config) error {
	// Initialize structured logger
	logger.Init()

	// 1. Connect to Database
	database.Connect()
	handlers.InitSession()

	// Initialize resource service
	resourcesvc.Init(database.DB)

	if cfg.Seed || os.Getenv("AUTO_SEED") == "true" {
		database.SeedInitialUser(database.DB)
	}

	// 2. Setup Template Engine
	engine := html.New("./views", ".html")
	if os.Getenv("ENV") != "production" {
		engine.Reload(true)
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
		Views:                   engine,
		EnableTrustedProxyCheck: true,
		TrustedProxies:          []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
		ProxyHeader:             fiber.HeaderXForwardedFor,
	})

	// 4. Middleware
	// Request ID middleware for tracing
	app.Use(requestid.New())

	// Logger with request ID
	app.Use(fiberlogger.New(fiberlogger.Config{
		Format: "[${time}] ${status} - ${latency} ${method} ${path} [${locals:requestid}]\n",
	}))

	// HTTPS enforcement middleware (only in production)
	app.Use(func(c *fiber.Ctx) error {
		if os.Getenv("ENV") == "production" {
			proto := c.Get("X-Forwarded-Proto")
			if proto == "" {
				proto = c.Protocol()
			}
			if proto != "https" {
				return c.Redirect("https://"+c.Hostname()+c.OriginalURL(), fiber.StatusMovedPermanently)
			}
		}
		return c.Next()
	})

	app.Use(helmet.New(helmet.Config{
		ContentSecurityPolicy:     "default-src 'self'; script-src 'self' 'unsafe-inline' https://unpkg.com https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data: https://github.com https://avatars.githubusercontent.com; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; upgrade-insecure-requests;",
		CrossOriginEmbedderPolicy: "unsafe-none",
		CrossOriginResourcePolicy: "cross-origin",
		XFrameOptions:             "DENY",
		ContentTypeNosniff:        "nosniff",
		XSSProtection:             "1; mode=block",
		ReferrerPolicy:            "same-origin",
	}))

	// Add Permissions-Policy header
	app.Use(func(c *fiber.Ctx) error {
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		return c.Next()
	})

	// CSRF Middleware
	isProduction := os.Getenv("ENV") == "production"
	app.Use(csrf.New(csrf.Config{
		KeyLookup:      "header:X-CSRF-Token",
		ContextKey:     "csrf",
		CookieName:     "csrf_token",
		CookieSameSite: "Lax",
		CookieSecure:   isProduction,
		CookieHTTPOnly: true,
		Expiration:     1 * time.Hour,
		SingleUseToken: false,
		Extractor: func(c *fiber.Ctx) (string, error) {
			// Check header first (for HTMX requests)
			if token := c.Get("X-CSRF-Token"); token != "" {
				return token, nil
			}
			// Check form field (for regular form submissions)
			if token := c.FormValue("_csrf"); token != "" {
				return token, nil
			}
			return "", csrf.ErrTokenNotFound
		},
	}))

	// Maximum absolute session lifetime (7 days)
	const maxSessionLifetime = 7 * 24 * time.Hour

	// Session and Flash middleware
	app.Use(func(c *fiber.Ctx) error {
		sess, err := handlers.Store.Get(c)
		if err == nil {
			// Check absolute session timeout
			if createdAt := sess.Get("session_created_at"); createdAt != nil {
				if created, ok := createdAt.(int64); ok {
					if time.Since(time.Unix(created, 0)) > maxSessionLifetime {
						// Session expired - destroy and redirect to login
						if err := sess.Destroy(); err != nil {
							log.Printf("failed to destroy expired session: %v", err)
						}
						if c.Get("Accept") == "application/json" {
							return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Session expired"})
						}
						return c.Next() // Continue without user context
					}
				}
			}

			if userID := sess.Get("user_id"); userID != nil {
				var user models.User
				if err := database.DB.Preload("Memberships.Organization").First(&user, userID).Error; err == nil {
					// Check if password was changed after session was created
					// If so, invalidate this session (force logout on password reset)
					if createdAt := sess.Get("session_created_at"); createdAt != nil {
						if created, ok := createdAt.(int64); ok {
							sessionTime := time.Unix(created, 0)
							// PasswordChangedAt is zero value for users who never changed password
							if !user.PasswordChangedAt.IsZero() && user.PasswordChangedAt.After(sessionTime) {
								// Session is stale - password was changed after this session was created
								if err := sess.Destroy(); err != nil {
									log.Printf("failed to destroy stale session: %v", err)
								}
								// Continue request without user context (will redirect to login on protected routes)
								return c.Next()
							}
						}
					}
					c.Locals("UserID", userID)
					c.Locals("User", user)
				}
			}

			flashType := sess.Get("flash_type")
			flashMessage := sess.Get("flash_message")
			if flashType != nil && flashMessage != nil {
				c.Locals("Flash", handlers.Flash{
					Type:    flashType.(string),
					Message: flashMessage.(string),
				})
				sess.Delete("flash_type")
				sess.Delete("flash_message")
				if err := sess.Save(); err != nil {
					log.Printf("Error saving session in middleware: %v", err)
				}
			}
		}

		c.Locals("CSRFToken", c.Locals("csrf"))
		c.Locals("LatestVersion", cfg.Version)
		return c.Next()
	})

	// 5. Static Files
	app.Static("/public", "./public")
	app.Static("/favicon.ico", "./public/favicon.ico")
	app.Static("/robots.txt", "./public/robots.txt")
	app.Static("/.well-known/security.txt", "./public/.well-known/security.txt")

	// SEO Routes
	app.Get("/sitemap.xml", handlers.GenerateSitemap)

	// Swagger UI / API Docs
	app.Get("/swagger/*", swagger.HandlerDefault)
	app.Get("/api-docs", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/index.html", fiber.StatusMovedPermanently)
	})

	// Rate limiter for search endpoints
	searchLimiter := limiter.New(limiter.Config{
		Max:        60,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).SendString("Too many search requests. Please try again later.")
		},
	})

	// 6. Routes
	app.Get("/", handlers.Home)
	app.Get("/search", searchLimiter, handlers.Search)
	app.Get("/packs", handlers.GetPacks)
	app.Get("/jobs", handlers.GetJobs)
	app.Get("/registries", handlers.GetRegistries)
	app.Get("/docs", handlers.GetDocs)
	app.Get("/docs/:page", handlers.GetDocsPage)
	app.Get("/docs/:page/:subpage", handlers.GetDocsPage)
	app.Get("/about", handlers.GetAbout)

	// Request Routes (community pack/job requests)
	voteLimiter := limiter.New(limiter.Config{
		Max:        30,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).SendString("Too many vote requests. Please try again later.")
		},
	})

	app.Get("/requests", handlers.GetRequests)
	app.Get("/requests/new", handlers.RequireAuth, handlers.GetNewRequest)
	app.Post("/requests/new", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostNewRequest)
	app.Get("/requests/:id", handlers.GetRequest)
	app.Get("/requests/:id/edit", handlers.RequireAuth, handlers.GetEditRequest)
	app.Post("/requests/:id/edit", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostEditRequest)
	app.Delete("/requests/:id", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.DeleteUserRequest)
	app.Post("/requests/:id/vote", handlers.RequireAuth, voteLimiter, handlers.ToggleRequestVote)
	app.Post("/requests/:id/github-issue", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.RetryGitHubIssue)

	// Auth Routes with rate limiter
	authLimiter := limiter.New(limiter.Config{
		Max:        5,
		Expiration: 15 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).SendString("Too many requests. Please try again later.")
		},
	})

	app.Get("/login", handlers.GetLogin)
	app.Post("/login", authLimiter, handlers.PostLogin)
	app.Get("/signup", handlers.GetSignup)
	app.Post("/signup", authLimiter, handlers.PostSignup)
	app.Post("/logout", handlers.Logout)
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
	admin.Get("/settings", handlers.GetAdminSettings)
	admin.Post("/settings", handlers.PostAdminSettings)
	admin.Get("/requests", handlers.GetAdminRequests)
	admin.Post("/requests/:id/status", handlers.PostUpdateRequestStatus)
	admin.Delete("/requests/:id", handlers.DeleteRequest)
	admin.Post("/requests/sync", handlers.PostSyncGitHubRequests)
	admin.Get("/audit", handlers.GetAdminAudit)
	admin.Get("/errors", handlers.GetAdminErrors)
	// Admin membership management
	admin.Post("/organizations/:id/members/add", handlers.PostAdminAddMember)
	admin.Delete("/organizations/:id/members/:member_id", handlers.PostAdminRemoveMember)
	admin.Post("/organizations/:id/members/:member_id/role", handlers.PostAdminChangeMemberRole)
	admin.Delete("/users/:id/memberships/:org_id", handlers.DeleteAdminUserMembership)

	// Resource Routes

	// CRUD Operations (resource_crud.go)
	app.Get("/new", handlers.RequireAuth, handlers.GetNewResource)
	app.Post("/new", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostNewResource)
	app.Get("/resource/:id/new-version", handlers.RequireAuth, handlers.GetNewVersion)
	app.Post("/resource/:id/version", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostNewVersion)
	app.Get("/:username/:resourcename/edit", handlers.RequireAuth, handlers.GetEditResource)
	app.Post("/resource/:id/edit", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostEditResource)
	app.Delete("/resource/:id", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.DeleteResource)
	app.Post("/resource/:id/retry-fetch", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostRetryFetch)

	// Webhook Handling (resource_webhooks.go)
	app.Get("/new/my-repos", handlers.RequireAuth, handlers.GetMyRepos)
	app.Get("/new/fetch-info", handlers.RequireAuth, handlers.FetchInfo)
	app.Post("/resource/:id/webhook", handlers.HandleWebhook)
	app.Post("/resource/:id/webhook/reset", handlers.RequireAuth, handlers.RequireVerifiedEmail, handlers.PostResetWebhookSecret)
	app.Get("/resource/:id/fetch-readme", handlers.FetchReadme)

	// Social Features (resource_social.go)
	app.Post("/resource/:id/star", handlers.RequireAuth, handlers.ToggleStar)

	// Dev Routes (disabled in production)
	if os.Getenv("ENV") != "production" {
		app.Get("/api/dev/packs", handlers.GetDevelopmentPacks)
	}

	// API Routes (must be before catch-all /:username routes)
	app.Get("/v1/recent", handlers.ListRecentAPI)
	app.Get("/v1/packs", handlers.ListAllPacksAPI)
	app.Get("/v1/packs/search", searchLimiter, handlers.SearchPacksAPI)
	app.Get("/v1/registries", handlers.ListUserRegistriesAPI)
	app.Get("/v1/registries/search", searchLimiter, handlers.SearchRegistriesAPI)
	app.Get("/v1/jobs", handlers.ListAllJobsAPI)
	app.Get("/v1/jobs/search", searchLimiter, handlers.SearchJobsAPI)

	// Webhook Routes
	app.Post("/webhooks/github/issues", handlers.HandleGitHubIssueWebhook)

	// Namespaced Routes (catch-all, must be last)
	app.Get("/:username", handlers.GetUserProfile)
	app.Get("/:username/:resourcename", handlers.GetResource)
	app.Get("/:username/:resourcename/v", handlers.GetResourceVersion)
	app.Get("/:username/:resourcename/raw", handlers.GetRawResource)
	app.Get("/:username/:resourcename/v/:version/raw", handlers.GetRawResourceVersion)

	// 7. Start Server
	port := cfg.Port
	if port == "" {
		port = "3000"
	}
	return app.Listen(":" + port)
}
