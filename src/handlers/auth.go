package handlers

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"automation-developer-guide/src/config"
	"automation-developer-guide/src/database"
	"automation-developer-guide/src/models"
	"automation-developer-guide/src/ondc"
	"automation-developer-guide/src/utils"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/oauth2"
)

// HandleHealth serves as a health check for the application
func HandleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok", "service": "automation-developer-guide"})
}

// HandleMe returns the authenticated user's details in JSON format
func HandleMe(c *fiber.Ctx) error {
	// User is already authenticated via middleware
	username := c.Locals("username").(string)
	email, _ := c.Locals("email").(string)
	userID, _ := c.Locals("user_id").(string)
	avatarURL, _ := c.Locals("avatar_url").(string)
	firstName, _ := c.Locals("first_name").(string)
	lastName, _ := c.Locals("last_name").(string)
	authProvider, _ := c.Locals("auth_provider").(string)
	orgID, _ := c.Locals("org_id").(string)
	orgName, _ := c.Locals("org_name").(string)
	user := fiber.Map{
		// githubId holds our own user ID; the name stays until the website stops reading it.
		"githubId":         userID,
		"username":         username,
		"email":            email,
		"avatarUrl":        avatarURL,
		"firstName":        firstName,
		"lastName":         lastName,
		"authProvider":     authProvider,
		"organisationId":   orgID,
		"organisationName": orgName,
	}
	if authProvider == config.LoginProviderONDC {
		addONDCProfile(user, userID)
	}
	return c.JSON(fiber.Map{"ok": true, "user": user})
}

// addONDCProfile adds what NP Portal sent at the last login (all organisations,
// subscriber IDs). It comes from the database, not the token, so the token stays
// small. If the lookup fails, the token's details are still returned.
func addONDCProfile(user fiber.Map, userID string) {
	id, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return
	}
	var u models.User
	if err := database.FindOne("users", bson.M{"_id": id}, &u); err != nil {
		log.Printf("loading ONDC profile for %s: %v", userID, err)
		return
	}
	orgs := make([]fiber.Map, 0, len(u.Organisations))
	for _, o := range u.Organisations {
		subscriberIDs := o.SubscriberIDs
		if subscriberIDs == nil {
			subscriberIDs = []string{}
		}
		orgs = append(orgs, fiber.Map{
			"organisationId":   o.OrganisationID,
			"organisationName": o.OrganisationName,
			"networkStatus":    o.NetworkStatus,
			"participantId":    o.ParticipantID,
			"npType":           o.NPType,
			"isRepresentative": o.IsRepresentative,
			"status":           o.Status,
			"subscriberIds":    subscriberIDs,
		})
	}
	user["ondcUserId"] = u.ONDCUserID
	user["ondcStatus"] = u.ONDCStatus
	user["organisations"] = orgs
	if !u.LastLoginAt.IsZero() {
		user["lastLoginAt"] = u.LastLoginAt
	}
}

// HandleLogin initiates the OAuth flow
func HandleLogin(c *fiber.Ctx) error {
	// Prevent caching of the login initiation to ensure fresh state
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	// ONDC's login link takes no state or return address: ONDC always sends the
	// user to the callback registered for this client ID.
	if config.LoginProvider == config.LoginProviderONDC {
		return c.Redirect(ondc.LoginURL(config.ONDCPortalBaseURL, config.ONDCClientID), fiber.StatusSeeOther)
	}

	// 1. Generate a random state string to prevent CSRF
	state, err := utils.GenerateRandomState()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate state")
	}

	// 2. Save state in a short-lived cookie
	samesite, secure := getCookieSettings()

	c.Cookie(&fiber.Cookie{
		Name:     config.StateKey,
		Value:    state,
		Expires:  time.Now().Add(10 * time.Minute),
		HTTPOnly: true,
		Secure:   secure,
		SameSite: samesite,
		Path:     "/",
	})

	// 3. Redirect user to GitHub
	url := config.OauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
	return c.Redirect(url, fiber.StatusSeeOther)
}

// HandleCallback processes the response from GitHub
func HandleCallback(c *fiber.Ctx) error {
	samesite, secure := getCookieSettings()

	// 1. Validate State (CSRF Protection)
	storedState := c.Cookies(config.StateKey)
	if storedState == "" {
		return c.Status(fiber.StatusBadRequest).SendString("State cookie not found")
	}

	queryState := c.Query("state")
	if queryState != storedState {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid state parameter")
	}

	// Remove state from session after validation
	c.Cookie(&fiber.Cookie{
		Name:     config.StateKey,
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		MaxAge:   -1,
		HTTPOnly: true,
		Secure:   secure,
		SameSite: samesite,
		Path:     "/",
	})

	// 2. Exchange authorization code for access token
	code := c.Query("code")
	token, err := config.OauthConfig.Exchange(context.Background(), code)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to exchange token: " + err.Error())
	}

	// 3. Fetch User Info from GitHub
	client := config.OauthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to get user info: " + err.Error())
	}
	defer resp.Body.Close()

	// Use a temporary struct to decode GitHub response to avoid type mismatch
	// between GitHub's numeric "id" and MongoDB's ObjectID "id"
	var githubUser struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Email     string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&githubUser); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to parse user info")
	}

	user := models.User{
		Login:     githubUser.Login,
		Name:      githubUser.Name,
		AvatarURL: githubUser.AvatarURL,
		Email:     githubUser.Email,
	}

	// If email is empty, fetch it from the emails endpoint
	if user.Email == "" {
		emailResp, err := client.Get("https://api.github.com/user/emails")
		if err == nil {
			defer emailResp.Body.Close()
			var emails []struct {
				Email    string `json:"email"`
				Primary  bool   `json:"primary"`
				Verified bool   `json:"verified"`
			}
			if err := json.NewDecoder(emailResp.Body).Decode(&emails); err == nil {
				for _, e := range emails {
					if e.Primary && e.Verified {
						user.Email = e.Email
						break
					}
				}
			}
		}
	}

	// Store or Update User in MongoDB
	// Check existence using Email instead of Login (Username)
	filter := bson.M{"email": user.Email}
	update := bson.M{"$set": user}

	var existingUser models.User
	var userID string

	err = database.FindOne("users", filter, &existingUser)
	if err == mongo.ErrNoDocuments {
		// User does not exist, create new
		result, err := database.CreateOne("users", user)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create user in db")
		}
		userID = result.InsertedID.(primitive.ObjectID).Hex()
	} else if err == nil {
		// User exists, update details
		if _, err := database.UpdateOne("users", filter, update); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update user in db")
		}
		userID = existingUser.ID.Hex()
	} else {
		return c.Status(fiber.StatusInternalServerError).SendString("Database error: " + err.Error())
	}

	// 4. Generate JWT
	firstName, lastName := splitName(user.Name)
	jwtToken, err := utils.GenerateJWT(userID, user.Email, user.Login, user.AvatarURL, firstName, lastName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate token")
	}

	return redirectWithExchangeCode(c, jwtToken)
}

// redirectWithExchangeCode hands the Workbench token to the website the same way
// for every login provider: store it behind a short-lived one-time code and send
// the browser to the website with that code.
func redirectWithExchangeCode(c *fiber.Ctx, jwtToken string) error {
	exchangeCode, err := utils.GenerateRandomState()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate exchange code")
	}

	codeData := models.ExchangeCode{
		Code:      exchangeCode,
		JWTToken:  jwtToken,
		CreatedAt: time.Now(),
	}

	if _, err := database.CreateOne("exchange_codes", codeData); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to store exchange code")
	}

	redirectURL := config.ClientURL
	if strings.Contains(redirectURL, "?") {
		redirectURL += "&code=" + exchangeCode
	} else {
		redirectURL += "?code=" + exchangeCode
	}

	return c.Redirect(redirectURL, fiber.StatusSeeOther)
}

// HandleLoginProvider tells the website which login is on, so the button text
// always matches LOGIN_PROVIDER.
func HandleLoginProvider(c *fiber.Ctx) error {
	// Not cached: after a switch the button must change on the next page load.
	c.Set("Cache-Control", "no-cache")
	return c.JSON(fiber.Map{"provider": config.LoginProvider})
}

const ondcLoginFailedMessage = "We could not complete Login via ONDC. Please try again or contact support."

// HandleONDCCallback is where ONDC sends the browser after login. It swaps the
// one-time code for the user's details, saves the user and logs them in.
func HandleONDCCallback(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	ref := newRef()
	fail := func(status int, reason string) error {
		// ref ties the message the user sees to this log line. Never log the code or the secret.
		log.Printf("ondc login failed ref=%s: %s", ref, reason)
		return c.Status(status).SendString(ondcLoginFailedMessage + " (ref " + ref + ")")
	}

	if config.LoginProvider != config.LoginProviderONDC {
		return fail(fiber.StatusNotFound, "LOGIN_PROVIDER is not ondc")
	}
	code := c.Query("code")
	if !ondc.ValidCode(code) {
		return fail(fiber.StatusBadRequest, "missing or malformed code")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := ondc.NewClient(config.ONDCAPIBaseURL, config.ONDCClientID, config.ONDCClientSecret).Exchange(ctx, code)
	if err != nil {
		status := fiber.StatusBadGateway
		if e, ok := ondc.IsExchangeError(err); ok {
			switch e.Kind {
			case ondc.KindInvalidCode:
				status = fiber.StatusBadRequest
			case ondc.KindRateLimited:
				status = fiber.StatusTooManyRequests
			}
		}
		return fail(status, err.Error())
	}

	user, err := saveONDCUser(info)
	if err != nil {
		return fail(fiber.StatusInternalServerError, "saving user: "+err.Error())
	}

	// All organisations are saved on the user. Until Product decides how a user
	// with several picks one, the first goes into the token.
	extra := map[string]interface{}{"auth_provider": config.LoginProviderONDC, "ondc_user_id": info.UserID}
	if len(info.Organisations) > 0 {
		org := info.Organisations[0]
		extra["org_id"] = org.OrgID()
		extra["org_name"] = org.OrganisationName
	}
	// The website links sessions and reports to username, so it must be unique
	// and permanent: the ONDC user ID. The header shows first and last name.
	username := strconv.FormatInt(info.UserID, 10)
	jwtToken, err := utils.GenerateJWTWithExtra(user.ID.Hex(), info.Email, username, "", info.FirstName, info.LastName, extra)
	if err != nil {
		return fail(fiber.StatusInternalServerError, "generating token")
	}

	return redirectWithExchangeCode(c, jwtToken)
}

// newRef returns a short random tag for one login attempt.
func newRef() string {
	r, err := utils.GenerateRandomState()
	if err != nil || len(r) < 8 {
		return "noref"
	}
	return r[:8]
}

// saveONDCUser finds the user by ONDC user ID, or else a GitHub-only user with
// the same email (so they keep their notes and preferences), and saves the
// latest details from ONDC.
func saveONDCUser(info *ondc.UserInfo) (*models.User, error) {
	orgs := make([]models.UserOrganisation, 0, len(info.Organisations))
	for _, o := range info.Organisations {
		orgs = append(orgs, models.UserOrganisation{
			OrganisationID:   o.OrgID(),
			OrganisationName: o.OrganisationName,
			NetworkStatus:    string(o.NetworkStatus),
			ParticipantID:    o.ParticipantID,
			NPType:           o.NPType,
			IsRepresentative: o.IsRepresentative,
			Status:           o.Status,
			SubscriberIDs:    o.SubscriberIDs,
		})
	}
	set := bson.M{
		"auth_provider": config.LoginProviderONDC,
		"ondc_user_id":  info.UserID,
		"email":         info.Email,
		"first_name":    info.FirstName,
		"last_name":     info.LastName,
		"name":          strings.TrimSpace(info.FirstName + " " + info.LastName),
		"ondc_status":   info.Status,
		"organisations": orgs,
		"last_login_at": time.Now().UTC(),
	}

	var existing models.User
	filter := bson.M{"ondc_user_id": info.UserID}
	err := database.FindOne("users", filter, &existing)
	if err == mongo.ErrNoDocuments && info.Email != "" {
		// Only a row with no ONDC ID: never take over another ONDC user's row.
		filter = bson.M{"email": info.Email, "ondc_user_id": bson.M{"$exists": false}}
		err = database.FindOne("users", filter, &existing)
	}
	switch {
	case err == nil:
		if _, err := database.UpdateOne("users", bson.M{"_id": existing.ID}, bson.M{"$set": set}); err != nil {
			return nil, err
		}
		return &existing, nil
	case err == mongo.ErrNoDocuments:
		set["_id"] = primitive.NewObjectID()
		if _, err := database.CreateOne("users", set); err != nil {
			return nil, err
		}
		return &models.User{ID: set["_id"].(primitive.ObjectID)}, nil
	default:
		return nil, err
	}
}

// HandleExchangeToken exchanges a short-lived code for a JWT token
func HandleExchangeToken(c *fiber.Ctx) error {
	var body struct {
		Code string `json:"code"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if body.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "code is required"})
	}

	// 1. Find the exchange code in DB
	filter := bson.M{"code": body.Code}
	var codeData models.ExchangeCode

	err := database.FindOne("exchange_codes", filter, &codeData)
	if err == mongo.ErrNoDocuments {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired code"})
	} else if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}

	// 2. Check for expiration (e.g., 5 minutes)
	if time.Since(codeData.CreatedAt) > 5*time.Minute {
		database.DeleteOne("exchange_codes", filter)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "code has expired"})
	}

	// 3. Success! Return the JWT and delete the exchange code (one-time use)
	defer database.DeleteOne("exchange_codes", filter)

	return c.JSON(fiber.Map{
		"ok":    true,
		"token": codeData.JWTToken,
	})
}

func splitName(name string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(name), " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return name, ""
}

func getCookieSettings() (string, bool) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production") {
		return "None", true
	}

	return "Lax", false
}
