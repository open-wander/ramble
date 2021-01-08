package router

import (
	"github.com/nsreg/rmbl/controller"
	"github.com/nsreg/rmbl/pkg/middleware"
	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
)

var db *gorm.DB

// Setup Func
func Setup() *gin.Engine {
	router := gin.New()

	// Middlewares
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(middleware.CORS())

	home := router.Group("/")
	{
		home.GET("/", controller.HomePage)
	}

	// Non-protected routes
	repos := router.Group("/api/v1/repo")
	{
		repos.GET("/", controller.GetRepos)
		repos.GET("/:name", controller.GetRepo)
	}

	// Protected routes
	// For authorized access, group protected routes using gin.BasicAuth() middleware
	// gin.Accounts is a shortcut for map[string]string

	authorized := router.Group("/admin", gin.BasicAuth(gin.Accounts{}))

	// /admin/dashboard endpoint is now protected
	authorized.GET("/dashboard", controller.Dashboard)
	authorized.POST("/createrepo", controller.CreateRepo)
	authorized.PUT("/updaterepo", controller.UpdateRepo)
	authorized.DELETE("/deleterepo", controller.DeleteRepo)

	return router
}
