package main

import (
	"log"
	"os"
	"strings"

	"automation-developer-guide/src/config"
	"automation-developer-guide/src/database"
	"automation-developer-guide/src/routes"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	// Initialize configuration
	config.Load()

	// Connect to MongoDB
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" && os.Getenv("ENV") == "development" {
		mongoURI = "mongodb://localhost:27017"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "developer_guide_db"
	}
	if err := database.Connect(mongoURI, dbName); err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}

	// 1.5 Ensure TTL index for exchange codes (expires after 5 minutes)
	if err := database.EnsureTTLIndex("exchange_codes", "created_at", 300); err != nil {
		log.Println("Warning: Failed to create TTL index for exchange_codes:", err)
	}

	if err := database.EnsureUniqueIndex("scenario_preferences", "user_id"); err != nil {
		log.Println("Warning: Failed to create unique index for scenario_preferences:", err)
	}
	
	if err := database.EnsureCommentIndexes(); err != nil {
		log.Println("Warning: Failed to create indexes for comments:", err)
	}

	if err := database.EnsureUserIndexes(); err != nil {
		log.Println("Warning: Failed to create indexes for users:", err)
	}

	if err := database.EnsureMCPKeyIndexes(); err != nil {
		log.Println("Warning: Failed to create indexes for mcp_keys:", err)
	}

	// Redis shared with the MCP server, so regenerate and revoke can clear its cached key checks.
	if config.RedisHost == "" {
		log.Println("Warning: REDIS_HOST is not set; regenerated or revoked MCP keys keep working until the MCP's cache expires")
	} else if err := database.ConnectRedis(config.RedisHost, config.RedisPort, config.RedisUsername, config.RedisPassword, config.MCPRedisKeyPrefix); err != nil {
		log.Println("Warning: Redis not reachable yet; it will be retried on each key change:", err)
	}

	app := fiber.New(fiber.Config{
		// consent_ip must record the user's address, not the ingress's, or
		// section 4 of the MCP consent document ("we record your IP address")
		// becomes a false statement.
		//
		// Safe here only because the service is reached exclusively through the
		// ALB/ingress, which overwrites this header. If anything is ever given
		// a direct route to this process, X-Forwarded-For becomes
		// client-spoofable and this line must be revisited.
		ProxyHeader: fiber.HeaderXForwardedFor,
	})

	// CORS: allow frontend origin (credentials for cookies/session)
	corsCfg := cors.Config{
		AllowOrigins:     config.ClientURL,
		AllowCredentials: true,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		// cookie fix
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production") {
		corsCfg.AllowOrigins = "https://workbench.ondc.tech"
	}
	app.Use(cors.New(corsCfg))

	// Routes
	routes.Setup(app)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server running at http://localhost:%s", port)
	log.Fatal(app.Listen(":" + port))
}
