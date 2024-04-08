package controllers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"rmbl/api/repositories"
	"rmbl/models"
	"rmbl/pkg/apperr"
	"rmbl/pkg/authhelpers"
	"rmbl/pkg/database"
	"rmbl/pkg/helpers"
	appvalid "rmbl/pkg/validator"

	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RepoRoutes registers the routes on the provided fiber.Router.
// It takes a fiber.Router as a parameter and adds the necessary routes for repository operations.
// The routes include getting all repositories, getting repositories by organization,
// getting a specific repository, updating a repository, creating a new repository,
// and deleting a repository.
func RepoRoutes(route fiber.Router) {
	route.Get("/", GetAllRepositories)
	route.Get("/:org", GetOrgRepositories)
	route.Get("/:org/:reponame/*", GetRepository)
	route.Put("/:org/:reponame", authhelpers.Protected(), UpdateRepository)
	route.Post("/:org", authhelpers.Protected(), NewRepository)
	route.Delete("/:org/:reponame", authhelpers.Protected(), DeleteRepository)
}

// GetAllRepositories is a handler function that retrieves all repositories based on the provided search query, order, offset, and limit.
// It takes a fiber.Ctx object as a parameter and returns an error.
// It parses the context for the parameters or uses default values.
// It returns the retrieved repositories as JSON data.
func GetAllRepositories(c *fiber.Ctx) error {
	// Parse the context for the params or use defaults.
	search := strings.ToLower(c.Query("search"))

	order := strings.ToUpper(c.Query("order", "DESC"))

	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil {
		return fmt.Errorf("unable to parse 'offset': %w", err)
	}

	limit, err := strconv.Atoi(c.Query("limit", "25"))
	if err != nil {
		return fmt.Errorf("unable to parse 'limit': %w", err)
	}
	reposervice, err := repositories.NewRepoService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	// Call the repository layer for the data
	data := reposervice.GetAllRepositories(order, search, limit, offset)

	return c.Status(200).JSON(&data)
}

// GetOrgRepositories is a handler function that retrieves the repositories of an organization.
// It takes a fiber.Ctx object as a parameter and returns an error.
// The function parses the context for the organization name and other query parameters.
// It then calls the repository layer to fetch the data and returns it as a JSON response.
func GetOrgRepositories(c *fiber.Ctx) error {
	// Parse the context for the params or use defaults.
	orgname := strings.ToLower(c.Params("org"))
	search := strings.ToLower(c.Query("search"))
	order := strings.ToUpper(c.Query("order", "DESC"))

	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil {
		return fmt.Errorf("unable to parse 'offset': %w", err)
	}

	limit, err := strconv.Atoi(c.Query("limit", "25"))
	if err != nil {
		return fmt.Errorf("unable to parse 'limit': %w", err)
	}
	reposervice, err := repositories.NewRepoService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}

	helperservice, err := helpers.NewHelperService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	orgid, err := helperservice.GetOrganizationIDByOrgName(orgname)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"Status": "Error", "Message": "Record not Found"})
	}
	// Call the repository layer for the data
	data := reposervice.GetOrgRepositories(order, search, limit, offset, orgid)

	return c.Status(200).JSON(data)
}

// GetRepository is a handler function that retrieves information about a repository.
// It takes a fiber.Ctx object as a parameter and returns an error.
// The function first extracts the organization name and repository name from the request parameters.
// It then retrieves the organization ID based on the organization name.
// Next, it gets the user agent from the request to handle git redirects.
// After that, it creates a new instance of the repositories.RepoService using the database.DB connection.
// If an error occurs during the creation of the RepoService, it returns an internal server error response.
// Otherwise, it calls the GetRepository method of the RepoService to retrieve the repository data.
// If the user agent starts with "git", it constructs a redirect URL and redirects the request.
// Otherwise, it returns a JSON response with the repository data.
func GetRepository(c *fiber.Ctx) error {
	orgname := strings.ToLower(c.Params("org"))
	reponame := strings.ToLower(c.Params("reponame"))
	// Get Useragent from request as we need it to cater for git redirects
	useragent := string(c.Context().UserAgent())
	reposervice, err := repositories.NewRepoService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	data := reposervice.GetRepository(reponame, orgname)
	if strings.HasPrefix(useragent, "git") {
		p := "/" + c.Params("*") + "?" + string(c.Context().QueryArgs().QueryString())
		return c.Redirect(data.Data.URL+p, 302)
	} else {
		return c.JSON(fiber.Map{"Status": "Success", "Message": "Repository Found", "Data": data.Data})
	}
}

// NewRepository is a handler function that creates a new repository.
// It accepts a fiber.Ctx object as a parameter and returns an error.
// The function performs the following steps:
// 1. Validates the request header to ensure it is of type "application/json".
// 2. Retrieves the organization name and user token from the context.
// 3. Parses the request body into a models.Repository object.
// 4. Validates the repository object using a validator.
// 5. Checks the validity of the user token.
// 6. Compares the username with the organization name.
// 7. Creates a new repository using the repositories.NewRepoService function.
// 8. Returns the created repository as a JSON response.
func NewRepository(c *fiber.Ctx) error {
	c.Accepts("application/json")
	// Valid Header
	if !helpers.ValidRequestHeader(c) {
		return apperr.UnsupportedMediaType("Not Valid Content Type, Expect application/json")
	}

	reposervice, err := repositories.NewRepoService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	helperservice, err := helpers.NewHelperService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	repository := &models.Repository{}
	orgname := strings.ToLower(c.Params("org"))
	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	username := claims["username"].(string)
	is_siteadmin := claims["site_admin"].(bool)
	userid, err := helperservice.GetUserIDByUserName(username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure unable to create repository user not found"})
	}
	// check JSON input
	if err := c.BodyParser(repository); err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Review your repository input", "Data": err})
	}

	validate := appvalid.NewValidator()
	if validateerr := validate.Struct(repository); validateerr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"Status":  "validation-error",
			"Message": appvalid.ValidationErrors(validateerr),
		})
	}

	// Check for a valid token
	if !helpers.ValidToken(user_token, userid) {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid token id", "Data": nil})
	}
	// Check that the username matches the orgname
	if username != orgname {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}
	if username == orgname || is_siteadmin {
		createdRepository, err := reposervice.NewRepository(*repository, orgname, is_siteadmin)
		if err != nil {
			return fmt.Errorf("unable to create repository: %w", err)
		}
		repository = &createdRepository
	}
	return c.JSON(repository)
}

// UpdateRepository  is a handler function that updates a repository based on the provided organization name and repository name.
// It expects the request body to be in JSON format and validates the request header.
// The user must have a valid token and be authorized to update the repository.
// If the update is successful, it returns the updated repository as a JSON response.
// Otherwise, it returns an error message.
func UpdateRepository(c *fiber.Ctx) error {
	c.Accepts("application/json")
	// Valid Header
	if !helpers.ValidRequestHeader(c) {
		return apperr.UnsupportedMediaType("Not Valid Content Type, Expect application/json")
	}
	helperservice, err := helpers.NewHelperService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}

	var id uuid.UUID
	orgname := c.Params("org")
	reponame := c.Params("reponame")

	user_token := c.Locals("user").(*jwt.Token)
	claims := user_token.Claims.(jwt.MapClaims)
	username := claims["username"].(string)
	is_siteadmin := claims["site_admin"].(bool)
	userorg_id, _ := uuid.Parse(claims["userorg_id"].(string))
	id, err = helperservice.GetUserIDByUserName(username)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"Status": "Error", "Message": "Record not Found"})
	}
	orgid, err := helperservice.GetOrganizationIDByOrgName(orgname)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"Status": "Error", "Message": "Record not Found"})
	}
	updatedRepository := &models.Repository{}
	repository := &models.Repository{}
	if err := c.BodyParser(updatedRepository); err != nil {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Review your repository input", "Data": err})
	}

	validate := appvalid.NewValidator()
	if validateerr := validate.Struct(updatedRepository); validateerr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"Status":  "validation-error",
			"Message": appvalid.ValidationErrors(validateerr),
		})
	}

	if !helpers.ValidToken(user_token, id) {
		return c.Status(500).JSON(fiber.Map{"Status": "Error", "Message": "Invalid token id", "Data": nil})
	}
	if userorg_id != orgid || username != orgname {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}
	reposervice, err := repositories.NewRepoService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	if userorg_id == orgid || username == orgname || is_siteadmin {
		response, err := reposervice.UpdateRepository(orgid, reponame, *updatedRepository)
		if err != nil {
			return fmt.Errorf("unable to update repository: %w", err)
		}
		repository = &response

	}
	return c.JSON(repository)
}

// DeleteRepository is a handler function that deletes a repository.
// It takes a fiber.Ctx object as a parameter and returns an error.
// The function first retrieves the organization name and repository name from the request parameters.
// It then retrieves the user information from the context and checks if the user is authorized to delete the repository.
// If the user is authorized, it creates a new instance of the repositories.RepoService and calls its DeleteRepository method to delete the repository.
// Finally, it returns a JSON response indicating the status of the operation.
func DeleteRepository(c *fiber.Ctx) error {
	orgname := c.Params("org")
	reponame := c.Params("reponame")
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	username := claims["username"].(string)
	is_siteadmin := claims["site_admin"].(bool)
	userorg_id, _ := uuid.Parse(claims["userorg_id"].(string))

	helperservice, err := helpers.NewHelperService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}
	orgid, err := helperservice.GetOrganizationIDByOrgName(orgname)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"Status": "Error", "Message": "Record not Found"})
	}

	if userorg_id != orgid || username != orgname {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Status": "Error", "Message": "Unauthorized", "Data": nil})
	}

	reposervice, err := repositories.NewRepoService(database.DB)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Status": "Error", "Message": "Server failure"})
	}

	if userorg_id == orgid || username == orgname || is_siteadmin {
		err := reposervice.DeleteRepository(orgid, reponame)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("repository not found")
		} else if err != nil {
			return fmt.Errorf(err.Error())
		}
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"Status": "Success", "Message": reponame + " Deleted", "Data": nil})
}
