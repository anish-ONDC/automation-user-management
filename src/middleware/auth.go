package middleware

import (
	"automation-developer-guide/src/utils"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// IsAuthenticated checks if the user is logged in by parsing the JWT from the session cookie.
// This replaces the old AuthProxyMiddleware that made HTTP calls to the auth service.
func IsAuthenticated(c *fiber.Ctx) error {
	// 1. Get token from Authorization header
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	// 2. Extract token from "Bearer <token>"
	tokenParts := strings.Split(authHeader, " ")
	if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid authorization format"})
	}
	token := tokenParts[1]

	claims, err := utils.ParseJWT(token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	// Store username in locals for easy access in handlers
	if username, ok := claims["username"].(string); ok {
		c.Locals("username", username)
	}

	// Store user_id in locals
	if userID, ok := claims["user_id"].(string); ok {
		c.Locals("user_id", userID)
	}

	if email, ok := claims["email"].(string); ok {
		c.Locals("email", email)
	}

	if avatarURL, ok := claims["avatar_url"].(string); ok {
		c.Locals("avatar_url", avatarURL)
	}

	if firstName, ok := claims["first_name"].(string); ok {
		c.Locals("first_name", firstName)
	}

	if lastName, ok := claims["last_name"].(string); ok {
		c.Locals("last_name", lastName)
	}

	// Present only on tokens issued by Login via ONDC.
	if provider, ok := claims["auth_provider"].(string); ok {
		c.Locals("auth_provider", provider)
	}
	if orgID, ok := claims["org_id"].(string); ok {
		c.Locals("org_id", orgID)
	}
	if orgName, ok := claims["org_name"].(string); ok {
		c.Locals("org_name", orgName)
	}

	return c.Next()
}
