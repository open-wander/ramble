package authentication

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"rmbl/models"
	"rmbl/pkg/apperr"
	"rmbl/pkg/helpers"

	appconfig "rmbl/pkg/config"
	appvalid "rmbl/pkg/validator"

	jwt "github.com/form3tech-oss/jwt-go"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuthService represents the authentication service.
type AuthService struct {
	db *gorm.DB
}

// NewAuthService creates a new instance of AuthService with the provided database connection.
// It performs checks on the db parameter and returns an error if it is empty or if there is an error pinging the database.
// Parameters:
// - dbconn: The database connection to be used by the AuthService.
// Returns:
// - *AuthService: The newly created AuthService instance.
// - error: An error if the database connection is empty or if there is an error pinging the database.
func NewAuthService(dbconn *gorm.DB) (*AuthService, error) {
	// do some checks on the db parameter in case there's an error to return
	if dbconn == nil {
		return nil, errors.New("database connection cannot be empty")
	}
	sqlDB, _ := dbconn.DB()
	if sqlDBerr := sqlDB.Ping(); sqlDBerr != nil {
		return &AuthService{
			db: dbconn,
		}, sqlDBerr
	}

	return &AuthService{
		db: dbconn,
	}, nil
}

// SignUp registers a new user with the provided user signup data.
// It creates a new organization, hashes the password, and saves the user to the database.
// Returns the newly created user and any error encountered.
func (s *AuthService) SignUp(u models.UserSignupData) (models.NewUser, error) {
	org := new(models.Organization)
	uname := strings.ToLower(u.Username)
	hash, err := helpers.HashPassword(u.Password)
	if err != nil {
		return models.NewUser{}, fmt.Errorf("couldn't hash password error: %w", err)
	}

	org.OrgName = uname
	user := models.User{
		Email:        u.Email,
		Username:     uname,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Password:     hash,
		Organization: *org,
	}

	dbErr := s.db.Create(&user).Error
	if dbErr != nil {
		return models.NewUser{}, fmt.Errorf("unable to create user error: %w", dbErr)
	}

	newUser := &models.NewUser{
		ID:        user.ID,
		Email:     user.Email,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
	}
	return *newUser, err
}

// Login authenticates a user and generates a JWT token.
// It takes a UserLoginInput struct as input and returns a JWTToken and an error.
// The UserLoginInput struct contains the user's identity (email or username) and password.
// The function performs validation on the input data and checks if the user exists.
// If the user is found and the password is correct, a JWT token is generated and returned.
// The token contains claims such as the username, user ID, organization ID, site admin status, and expiration time.
// If any error occurs during the authentication process, an error is returned.
func (s *AuthService) Login(u models.UserLoginInput) (models.JWTToken, error) {
	type UserData struct {
		ID        uuid.UUID `json:"id"`
		Username  string    `json:"username"`
		Email     string    `json:"email"`
		Password  string    `json:"password"`
		SiteAdmin bool      `json:"-"`
	}

	var userdata UserData
	jwtToken := &models.JWTToken{}

	helperservice, err := helpers.NewHelperService(s.db)
	if err != nil {
		return *jwtToken, fmt.Errorf("message: server failure")
	}

	validate := appvalid.NewValidator()
	if validateerr := validate.Struct(u); validateerr != nil {
		return *jwtToken, fmt.Errorf("validation error in json")
	}
	identity := strings.ToLower(u.Identity)
	pass := u.Password
	user, err := helperservice.GetUserByEmail(identity)
	if err != nil {
		return *jwtToken, fmt.Errorf("error on email error: %w", err)
	}
	if user.Email == "" {
		return *jwtToken, fmt.Errorf("invalid username credentials error: %w", err)
	}

	userdata = UserData{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		SiteAdmin: user.SiteAdmin,
		Password:  user.Password,
	}

	if !helpers.CheckPasswordHash(pass, userdata.Password) {
		return *jwtToken, fmt.Errorf("invalid credentials")
	}

	orgid, err := helperservice.GetORGIDByUserid(userdata.ID)
	if err != nil {
		return *jwtToken, apperr.EntityNotFound("unable to find user")
	}

	token := jwt.New(jwt.SigningMethodHS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["username"] = userdata.Username
	claims["user_id"] = userdata.ID
	claims["userorg_id"] = orgid
	claims["site_admin"] = userdata.SiteAdmin
	claims["expires"] = time.Now().Add(time.Hour * 72).Unix()

	t, err := token.SignedString([]byte(appconfig.Config.Server.JWTSecret))
	if err != nil {
		return *jwtToken, fmt.Errorf("token signing error error: %w", err)
	}
	jwtToken.Token = t
	return *jwtToken, nil
}
