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
	repos := router.Group("/v1")
	{
		// Check that the endpoint implements Ramble registry API V1.
		repos.GET("/", controller.NotImplemented)
		// Catalogue
		// Retrieve a sorted, json list of repositories available in the registry.
		repos.GET("/_catalog", controller.NotImplemented)
		// Fetch the tags under the repository identified by name.
		repos.GET("/<name>/tags/list", controller.NotImplemented)
		// Manifests
		// Fetch the manifest identified by name and reference where reference can be a tag or digest
		repos.GET("/<name>/manifests/<reference>", controller.NotImplemented)
		// Put the manifest identified by name and reference where reference can be a tag or digest.
		repos.PUT("/<name>/manifests/<reference>", controller.NotImplemented)
		// Delete the manifest identified by name and reference. Note that a manifest can only be deleted by digest.
		repos.DELETE("/<name>/manifests/<reference>", controller.NotImplemented)
		// Blobs
		// Retrieve the blob from the registry identified by digest
		repos.GET("/<name>/blobs/<digest>", controller.NotImplemented)
		// Delete the blob identified by name and digest
		repos.DELETE("/<name>/blobs/<digest>", controller.NotImplemented)
		// Initiate a resumable blob upload. If successful, an upload location will be provided to complete the upload.
		repos.POST("/<name>/blobs/uploads/", controller.NotImplemented)
		// Retrieve status of upload identified by uuid.
		repos.GET("/<name>/blobs/uploads/<uuid>", controller.NotImplemented)
		// Upload a chunk of data for the specified upload.
		repos.PATCH("/<name>/blobs/uploads/<uuid>", controller.NotImplemented)
		// Complete the upload specified by uuid, optionally appending the body as the final chunk.
		repos.PUT("/<name>/blobs/uploads/<uuid>", controller.NotImplemented)
		// Cancel outstanding upload processes, releasing associated resources.
		repos.DELETE("/<name>/blobs/uploads/<uuid>", controller.NotImplemented)
	}
	return router
}
