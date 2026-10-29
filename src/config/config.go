package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

var (
	OauthConfig *oauth2.Config
	ClientURL   string
	// cookie fix
	CookieDomain string
	JWTSecret    string
	StateKey     = "oauth-state"

	// Login via ONDC. The client secret must stay server side (never logged,
	// never sent to the browser).
	LoginProvider     string
	ONDCPortalBaseURL string
	ONDCAPIBaseURL    string
	ONDCClientID      string
	ONDCClientSecret  string

	// MCP API keys. MCPKeyTTLDays must stay in step with section 3 of the MCP
	// consent document, which tells the user how long their key lasts; changing
	// one without the other makes the document untrue.
	MCPKeyTTLDays     int
	MCPConsentVersion string

	// MCPServiceToken optionally gates /mcp/verify. Empty means the endpoint is
	// reachable without it, which is the default so that local development and a
	// first MCP integration work without cross-repo secret coordination.
	// Deliberately no hardcoded fallback: a default shared secret in source is
	// worse than none, because it would silently "work" in production.
	MCPServiceToken string

	// Redis shared with the MCP server. When a key is regenerated or revoked we
	// delete the MCP's cached verify answer for the old key there, so section 3
	// of the consent document ("the old key stops working immediately") holds
	// even though the MCP caches verify results.
	RedisHost     string
	RedisPort     string
	RedisUsername string
	RedisPassword string

	// MCPRedisKeyPrefix must equal the MCP server's REDIS_KEY_PREFIX, or our
	// deletes miss its cache entries and revocation waits for their TTL.
	MCPRedisKeyPrefix string
)

const (
	LoginProviderGitHub = "github"
	LoginProviderONDC   = "ondc"
)

const (
	defaultMCPKeyTTLDays     = 90
	defaultMCPConsentVersion = "2026-10-v1"
	defaultRedisPort         = "6379"
	defaultMCPRedisKeyPrefix = "ondc-mcp"
)

func Load() {
	// 1. Load Environment Variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system environment variables")
	}

	JWTSecret = os.Getenv("JWT_SECRET")
	if JWTSecret == "" {
		JWTSecret = "supersecretkey"
	}

	// 3. Configure OAuth
	OauthConfig = &oauth2.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("GITHUB_REDIRECT_URL"),
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}

	// 3b. Login via ONDC (SSO Onboarding Guide v1.2). GitHub stays the default so
	// a deploy without the ONDC secrets keeps working until LOGIN_PROVIDER=ondc.
	LoginProvider = strings.ToLower(strings.TrimSpace(os.Getenv("LOGIN_PROVIDER")))
	switch LoginProvider {
	case LoginProviderGitHub, LoginProviderONDC:
	case "":
		LoginProvider = LoginProviderGitHub
	default:
		log.Printf("Warning: unknown LOGIN_PROVIDER %q, using github", LoginProvider)
		LoginProvider = LoginProviderGitHub
	}
	ONDCPortalBaseURL = strings.TrimRight(os.Getenv("ONDC_PORTAL_BASE_URL"), "/")
	ONDCAPIBaseURL = strings.TrimRight(os.Getenv("ONDC_API_BASE_URL"), "/")
	ONDCClientID = os.Getenv("ONDC_SSO_CLIENT_ID")
	ONDCClientSecret = os.Getenv("ONDC_SSO_CLIENT_SECRET")
	if LoginProvider == LoginProviderONDC && (ONDCPortalBaseURL == "" || ONDCAPIBaseURL == "" || ONDCClientID == "" || ONDCClientSecret == "") {
		log.Println("Warning: LOGIN_PROVIDER=ondc but an ONDC_* setting is missing; Login via ONDC will fail")
	}

	ClientURL = os.Getenv("CLIENT_URL")
	if ClientURL == "" && os.Getenv("ENV") == "development" {
		ClientURL = "http://localhost:3000"
	}

	// 4. MCP API keys
	MCPKeyTTLDays = defaultMCPKeyTTLDays
	if raw := os.Getenv("MCP_KEY_TTL_DAYS"); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days <= 0 {
			// Fall back rather than accept it: a zero or negative TTL would mint
			// keys that are already expired.
			log.Printf("Invalid MCP_KEY_TTL_DAYS %q, using %d", raw, defaultMCPKeyTTLDays)
		} else {
			MCPKeyTTLDays = days
		}
	}

	MCPConsentVersion = os.Getenv("MCP_CONSENT_VERSION")
	if MCPConsentVersion == "" {
		MCPConsentVersion = defaultMCPConsentVersion
	}

	MCPServiceToken = os.Getenv("MCP_SERVICE_TOKEN")
	if MCPServiceToken == "" {
		log.Println("MCP_SERVICE_TOKEN is not set; /mcp/verify is reachable without a service token")
	}

	// 5. Redis shared with the MCP server (optional; see RedisHost)
	RedisHost = os.Getenv("REDIS_HOST")
	RedisPort = envOr("REDIS_PORT", defaultRedisPort)
	RedisUsername = os.Getenv("REDIS_USERNAME")
	RedisPassword = os.Getenv("REDIS_PASSWORD")
	MCPRedisKeyPrefix = envOr("MCP_REDIS_KEY_PREFIX", defaultMCPRedisKeyPrefix)
}

// envOr returns the environment variable, or fallback when it is unset or empty.
func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
