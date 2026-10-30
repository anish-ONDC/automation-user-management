package routes

import (
	"automation-developer-guide/src/handlers"
	"automation-developer-guide/src/middleware"

	"github.com/gofiber/fiber/v2"
)

func Setup(app *fiber.App) {
	// ─── Public Routes ───────────────────────────────────────────
	app.Get("/", handlers.HandleHealth)
	app.Get("/health", handlers.HandleHealth)
	app.Get("/login", handlers.HandleLogin)
	app.Get("/auth/github/callback", handlers.HandleCallback)
	app.Get("/auth/ondc/callback", handlers.HandleONDCCallback)
	app.Get("/auth/provider", handlers.HandleLoginProvider)
	app.Post("/auth/exchange", handlers.HandleExchangeToken)

	// MCP API key verification -- called by the MCP server, never by a browser.
	// Not under `auth`: the caller holds the user's API key, not a JWT. Gated
	// instead by X-Service-Token when MCP_SERVICE_TOKEN is configured, and rate
	// limited because section 8 of the MCP consent document promises the user
	// that request limits are in place.
	//
	// X-Service-Token is deliberately absent from AllowHeaders in main.go, so a
	// browser preflight can never satisfy this route. Do not "fix" that.
	app.Post("/mcp/verify", middleware.MCPVerifyLimiter(), middleware.RequireServiceToken, handlers.HandleVerifyMCPKey)

	// Comments (Public)
	app.Get("api/comments", handlers.HandleGetComments)
	app.Get("api/comments/:id", handlers.HandleGetCommentByID)

	// Notes (Public)
	app.Get("api/notes", handlers.HandleGetNotes)

	// ─── Protected Routes (require authentication) ───────────────
	// Note: middleware applied per-route — app.Group("/", mw) in Fiber v2 is
	// equivalent to app.Use("/", mw) and would intercept /health.
	auth := middleware.IsAuthenticated

	// Auth
	app.Get("/auth/api/me", auth, handlers.HandleMe)
	app.Post("/auth/api/select-org", auth, handlers.HandleSelectOrganisation)

	// Comments
	app.Post("/api/comments", auth, handlers.HandleCreateComment)
	app.Put("/api/comments/:id", auth, handlers.HandleUpdateComment)
	app.Put("/api/comments/:id/resolve", auth, handlers.HandleResolveComment)
	app.Delete("/api/comments/:id", auth, handlers.HandleDeleteComment)

	// Notes
	app.Post("/api/notes", auth, handlers.HandleCreateNote)
	app.Put("/api/notes/:id", auth, handlers.HandleUpdateNote)
	app.Delete("/api/notes/:id", auth, handlers.HandleDeleteNote)

	// Scenario Preferences
	app.Get("/user/scenario-preferences", auth, handlers.HandleGetScenarioPreferences)
	app.Put("/user/scenario-preferences/:config_key", auth, handlers.HandleUpsertScenarioPreference)
	app.Delete("/user/scenario-preferences/:config_key", auth, handlers.HandleDeleteScenarioPreference)

	// MCP API keys (browser-facing, user-authenticated). The generate limiter
	// runs after `auth` so that user_id is in Locals for its key generator.
	app.Get("/user/mcp-key", auth, handlers.HandleGetMCPKeyStatus)
	app.Post("/user/mcp-key", auth, middleware.MCPGenerateLimiter(), handlers.HandleGenerateMCPKey)
	app.Delete("/user/mcp-key", auth, handlers.HandleRevokeMCPKey)
}
