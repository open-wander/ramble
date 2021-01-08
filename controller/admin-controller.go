package controller

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
	"github.com/nsreg/rmbl/model"
	"github.com/nsreg/rmbl/pkg/config"
	"github.com/nsreg/rmbl/pkg/database"
	"golang.org/x/oauth2"

	"github.com/gin-gonic/gin"
)

// Dashboard func
func Dashboard(c *gin.Context) {

	// Get user info from BasicAuth middleware
	// AuthUserKey is the cookie name for user credential in basic auth.
	user := c.MustGet(gin.AuthUserKey).(string)

	//Show some secret info
	c.JSON(http.StatusOK, gin.H{"Welcome: ": user})

}

// CreateRepo func
func CreateRepo(c *gin.Context) {
	fmt.Println("Endpoint Hit: createNewRepo")
	user := c.MustGet(gin.AuthUserKey).(string)
	db = database.GetDB()
	var createR CreateRep
	if err := c.ShouldBindJSON(&createR); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// var createRepo Repository
	createRepo, err := getRepoDetails(createR.Name)
	createRepo.CreatedBy = user
	if err != nil {
		fmt.Println("Github Repository Error")
		fmt.Println(err)
	} else if err := db.Create(&createRepo).Error; err != nil {
		fmt.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		// c.AbortWithStatus(404)
		return
	}
	c.JSON(200, createRepo)
}

// UpdateRepo function that updates the repository if called.
func UpdateRepo(c *gin.Context) {
	fmt.Println("Endpoint Hit: updateRepo")
	user := c.MustGet(gin.AuthUserKey).(string)
	db = database.GetDB()
	var updateR CreateRep
	if err := c.ShouldBindJSON(&updateR); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var updRepo Repository
	if err := db.Where("repo_name = ? ", updateR.Name).First(&updRepo).Error; err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updatedRepo, err := getRepoDetails(updateR.Name)
	updatedRepo.UpdatedBy = user
	if err != nil {
		fmt.Println("Github Repository Error")
		fmt.Println(err)
	} else if err := db.Where("repo_name = ? ", updateR.Name).Save(&updatedRepo).Error; err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, &updatedRepo)
}

// DeleteRepo func
func DeleteRepo(c *gin.Context) {
	fmt.Println("Endpoint Hit: deleteRepo")
	// user := c.MustGet(gin.AuthUserKey).(string)
	db = database.GetDB()
	var deleteR CreateRep
	if err := c.ShouldBindJSON(&deleteR); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	repoName := deleteR.Name
	var repo Repository
	// if err := db.Where("name = ? ", repoName).First(&repo).Error; err != nil {
	// 	log.Println(err)
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	return
	// }
	// repo.DeletedBy = user
	// if err := db.Where("name = ? ", repoName).Save(&repo).Error; err != nil {
	// 	log.Println(err)
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	// c.AbortWithStatus(404)
	// 	return
	// }

	if err := db.Select("RelVersions").Where("name = ? ", repoName).Delete(&repo).Error; err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		// c.AbortWithStatus(404)
		return
	}

	c.JSON(200, gin.H{"RepoName#": "" + repoName + " " + "deleted"})
}

func getRepoDetails(repoName string) (*Repository, error) {
	// Here you'd look up whether the given name matches a repository
	// in your database. I'm just doing a check against an empty string
	// for demonstration purposes.
	if repoName == "" {
		return nil, errors.New("name cannot be empty")
	}
	fmt.Println("repoName Value")
	fmt.Println(repoName)
	// Configuration section
	config := config.GetConfig()
	rmblGithubUser := config.Server.GithubUserName
	token := config.Server.GithubAuthToken
	if token == "" {
		log.Fatal("Unauthorized: No token present")
	}
	fmt.Println("git User")
	fmt.Println(rmblGithubUser)
	fmt.Println("git APIKey")
	fmt.Println(token)
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Get the Readme file location for the repository from github
	readme, _, readmErr := client.Repositories.GetReadme(ctx, rmblGithubUser, repoName, nil)
	if readmErr != nil {
		fmt.Println(readmErr)
	}
	fmt.Println("Readme")
	fmt.Println(readme)
	// Get the content of the3 readme for the repository
	readmeText, readmeTextErr := readme.GetContent()
	if readmeText == "" {
		log.Printf("failed to retrieve latestrelease for repository %s", repoName)
		readmeText = "No Readme in the Repository"
	}
	if readmeTextErr != nil {
		fmt.Println(readmeTextErr)
	}
	fmt.Println("Readme Content")
	fmt.Println(readmeText)
	// Get the latest release number for the repository from Github
	latestRelease, _, releasErr := client.Repositories.GetLatestRelease(ctx, rmblGithubUser, repoName)
	currentVersion := *latestRelease.TagName
	if releasErr != nil {
		fmt.Println(releasErr)
	}
	fmt.Println("Latest Release")
	fmt.Println(latestRelease)
	fmt.Println("Current Version")
	fmt.Println(currentVersion)
	// Get all the current releases for the project
	listrelease, _, listreleasErr := client.Repositories.ListReleases(ctx, rmblGithubUser, repoName, nil)
	if listreleasErr != nil {
		fmt.Println(listreleasErr)
	}
	fmt.Println("Release Lists")
	fmt.Println(listrelease)
	// Get the details of the repository from Gitgub.
	// This will be used in a number of places to populate the Repository struct.
	repositoryDetails, _, reptextErr := client.Repositories.Get(ctx, rmblGithubUser, repoName)
	description := *repositoryDetails.Description
	license := *repositoryDetails.License.Name
	url := *repositoryDetails.CloneURL
	stars := *repositoryDetails.StargazersCount
	fmt.Println("Repository Details")
	fmt.Println(repositoryDetails)
	fmt.Println("License Name")
	fmt.Println(license)
	fmt.Println("readme.GetHTMLURL()")
	fmt.Println(readme.GetHTMLURL())

	if reptextErr != nil {
		fmt.Println(reptextErr)
	}

	// At this point we've retrieved all the repository details from the source of truth
	// and we can now build the pointer to the Repository, populate its values, and return
	// it to the caller.
	response := &Repository{
		Name:           repoName,
		License:        license,
		Readme:         readmeText,
		URL:            url,
		CurrentVersion: currentVersion,
		Description:    description,
		Stars:          stars,
		RelVersions:    []model.RelVersion{},
	}
	for _, lr := range listrelease {
		response.RelVersions = append(response.RelVersions, model.RelVersion{
			ReleaseName: *lr.Name,
			TarURL:      *lr.TarballURL,
			ZipURL:      *lr.ZipballURL,
			URL:         *lr.URL,
			ReleaseTag:  *lr.TagName,
		})
	}
	// Return the pointer to the Repository that we just
	// built along with a nil error.
	return response, nil
}

// BasicAuth func
func BasicAuth() gin.HandlerFunc {

	return func(c *gin.Context) {
		auth := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)

		if len(auth) != 2 || auth[0] != "Basic" {
			respondWithError(401, "Unauthorized", c)
			return
		}
		payload, _ := base64.StdEncoding.DecodeString(auth[1])
		pair := strings.SplitN(string(payload), ":", 2)

		if len(pair) != 2 || !authenticateUser(pair[0], pair[1]) {
			respondWithError(401, "Unauthorized", c)
			return
		}

		c.Next()
	}
}

func authenticateUser(username, password string) bool {
	fmt.Println("Authentication Hit: ***")
	db = database.GetDB()
	var user model.User
	// fetch user from database. Here db.Client() is connection to your database. You will need to import your db package above.
	// This is just for example purpose
	err := db.Where(model.User{UserName: username, Password: password}).FirstOrCreate(&user)
	// err := db.Where(model.User{UserName: username, Password: password}).First(&user)
	if err.Error != nil {
		return false
	}
	return true
}

func respondWithError(code int, message string, c *gin.Context) {
	resp := map[string]string{"error": message}

	c.JSON(code, resp)
	c.Abort()
}
