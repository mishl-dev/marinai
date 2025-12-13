package main

import (
	"log"
	"marinai/pkg/bot"
	"marinai/pkg/cerebras"

	"marinai/pkg/config"
	"marinai/pkg/embedding"
	"marinai/pkg/memory"
	"marinai/pkg/surreal"
	"marinai/pkg/gemini"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	// Load config.yml
	cfg, err := config.LoadConfig("config.yml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Load .env for secrets
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	token := os.Getenv("DISCORD_TOKEN")
	cerebrasKey := os.Getenv("CEREBRAS_API_KEY")
	embeddingKey := os.Getenv("EMBEDDING_API_KEY")


	// Check each required environment variable individually for better error messages
	if token == "" {
		log.Fatal("Missing required environment variable: DISCORD_TOKEN")
	}
	if cerebrasKey == "" {
		log.Fatal("Missing required environment variable: CEREBRAS_API_KEY")
	}
	if embeddingKey == "" {
		log.Fatal("Missing required environment variable: EMBEDDING_API_KEY")
	}


	embeddingURL := os.Getenv("EMBEDDING_API_URL")
	if embeddingURL == "" {
		embeddingURL = "https://vector.mishl.dev/embed"
	}



	// Initialize Clients
	cerebrasClient := cerebras.NewClient(cerebrasKey, cfg.ModelSettings.Temperature, cfg.ModelSettings.TopP, nil)
	baseEmbeddingClient := embedding.NewClient(embeddingKey, embeddingURL)
	embeddingClient := embedding.NewCachedClient(baseEmbeddingClient, 500) // Cache up to 500 embeddings


	// Initialize Gemini Client - for image understanding and fast classification
	var geminiClient bot.GeminiClient
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey != "" {
		geminiClient = gemini.NewAdapter(geminiKey)
		log.Println("Gemini client initialized (Flash Lite - vision + classification)")
	} else {
		log.Println("GEMINI_API_KEY not set, image understanding and fast classification disabled")
	}

	// Initialize Memory Store (SurrealDB)
	surrealHost := os.Getenv("SURREAL_DB_HOST")
	surrealUser := os.Getenv("SURREAL_DB_USER")
	surrealPass := os.Getenv("SURREAL_DB_PASS")
	surrealNS := os.Getenv("SURREAL_DB_NAMESPACE")
	surrealDB := os.Getenv("SURREAL_DB_DATABASE")

	if surrealHost == "" {
		log.Fatal("Missing required environment variable: SURREAL_DB_HOST")
	}
	if surrealUser == "" {
		log.Fatal("Missing required environment variable: SURREAL_DB_USER")
	}
	if surrealPass == "" {
		log.Fatal("Missing required environment variable: SURREAL_DB_PASS")
	}
	if surrealNS == "" {
		surrealNS = "marin" // Default
	}
	if surrealDB == "" {
		surrealDB = "memory" // Default
	}

	// Add protocol if missing
	if len(surrealHost) > 0 && surrealHost[:4] != "ws://" && surrealHost[:5] != "wss://" {
		surrealHost = "wss://" + surrealHost + "/rpc"
	}

	log.Printf("Connecting to SurrealDB at %s (NS: %s, DB: %s)", surrealHost, surrealNS, surrealDB)
	surrealClient, err := surreal.NewClient(surrealHost, surrealUser, surrealPass, surrealNS, surrealDB)
	if err != nil {
		log.Fatalf("Failed to connect to SurrealDB: %v", err)
	}
	defer surrealClient.Close()

	memoryStore := memory.NewSurrealStore(surrealClient)

	// Initialize Bot Handler
	handler := bot.NewHandler(
		cerebrasClient,
		embeddingClient,
		geminiClient,
		memoryStore,
		cfg.Delays.MessageProcessing,
		cfg.MemorySettings.FactAgingDays,
		cfg.MemorySettings.FactSummarizationThreshold,
		cfg.MemorySettings.MaintenanceIntervalHours,
	)

	// Create Discord Session
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}
	// The core requirement: Masquerade as an Android client
	dg.Identify.Properties.Browser = "Discord Android"
	dg.Identify.Properties.Device = "Discord Android" // Consistency

	// Register Handlers
	dg.AddHandler(handler.MessageCreate)
	dg.AddHandler(handler.InteractionCreate)

	// Open Connection
	if err := dg.Open(); err != nil {
		log.Fatalf("Error opening connection: %v", err)
	}

	// Set Bot ID in handler (so it can ignore itself)
	handler.SetBotID(dg.State.User.ID)
	// Set Session in handler (for background tasks like loneliness check)
	handler.SetSession(&bot.DiscordSession{Session: dg})

	// Register slash commands (empty string = global, or specify guild ID for faster testing)
	// For production, use "" for global commands. For development, use a specific guild ID for instant updates.
	guildID := os.Getenv("DISCORD_GUILD_ID") // Optional: set this for faster command updates during development
	registeredCommands, err := bot.RegisterSlashCommands(dg, guildID)
	if err != nil {
		log.Fatalf("Error registering slash commands: %v", err)
	}

	// Cleanup function to unregister commands on shutdown
	defer func() {
		if err := bot.UnregisterSlashCommands(dg, guildID, registeredCommands); err != nil {
			log.Printf("Error unregistering slash commands: %v", err)
		}
	}()

	log.Println("Marin is now running. Press CTRL-C to exit.")

	// Set Custom Status
	err = dg.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name:  "Custom Status",
				Type:  discordgo.ActivityTypeCustom,
				State: "working on my next cosplay! ✨",
				Emoji: discordgo.Emoji{
					Name: "🧵",
				},
			},
		},
		Status: "online",
		AFK:    true,
	})
	if err != nil {
		log.Printf("Error setting custom status: %v", err)
	}

	// Wait for signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.Close()
}
