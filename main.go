package main

import (
	"log"
	"marinai/pkg/bot"
	"marinai/pkg/cache"
	"marinai/pkg/config"
	"marinai/pkg/embedding"
	"marinai/pkg/memory"
	"marinai/pkg/nvidia"
	"marinai/pkg/surreal"
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
	nvidiaKey := os.Getenv("NVIDIA_API_KEY")
	embeddingKey := os.Getenv("EMBEDDING_API_KEY")

	// Check each required environment variable individually for better error messages
	if token == "" {
		log.Fatal("Missing required environment variable: DISCORD_TOKEN")
	}
	if nvidiaKey == "" {
		log.Fatal("Missing required environment variable: NVIDIA_API_KEY")
	}
	if embeddingKey == "" {
		log.Fatal("Missing required environment variable: EMBEDDING_API_KEY")
	}

	embeddingURL := os.Getenv("EMBEDDING_API_URL")
	if embeddingURL == "" {
		embeddingURL = "https://vector.mishl.dev/embed"
	}

	// Initialize NVIDIA Client (handles chat, classification, and vision)
	llmClient := nvidia.NewClient(nvidiaKey, cfg.ModelSettings.Temperature, cfg.ModelSettings.TopP, nil)
	log.Println("NVIDIA client initialized (chat + vision)")

	baseEmbeddingClient := embedding.NewClient(embeddingKey, embeddingURL)
	embeddingClient := embedding.NewCachedClient(baseEmbeddingClient, 500) // Cache up to 500 embeddings

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

	// Initialize Redis Cache
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379" // Default for local dev
	}

	redisCache, err := cache.NewRedisCache(redisURL, "marinai")
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisCache.Close()
	log.Println("Redis cache connected")

	if err := bot.InitSandbox(); err != nil {
		log.Printf("Warning: Sandbox initialization failed: %v", err)
	}

	surrealStore := memory.NewSurrealStore(surrealClient)
	memoryStore := memory.NewCachedStore(surrealStore, redisCache)

	toolConfig := &bot.ToolConfig{
		WebSearch: bot.WebSearchConfig{
			Enabled:    cfg.Tools.WebSearch.Enabled,
			Backend:    cfg.Tools.WebSearch.Backend,
			MaxResults: cfg.Tools.WebSearch.MaxResults,
			Timeout:    cfg.Tools.WebSearch.Timeout,
		},
		WebScrape: bot.WebScrapeConfig{
			Enabled:     cfg.Tools.WebScrape.Enabled,
			MaxBodySize: cfg.Tools.WebScrape.MaxBodySize,
			Timeout:     cfg.Tools.WebScrape.Timeout,
		},
		Media: bot.MediaConfig{
			Enabled: cfg.Tools.Media.Enabled,
			Image: bot.ImageConfig{
				CompressionThreshold: cfg.Tools.Media.Image.CompressionThreshold,
				Quality:              cfg.Tools.Media.Image.Quality,
				MaxWidth:             cfg.Tools.Media.Image.MaxWidth,
				MaxHeight:            cfg.Tools.Media.Image.MaxHeight,
			},
			Audio: bot.AudioConfig{
				DockerImage:   cfg.Tools.Media.Audio.DockerImage,
				Language:      cfg.Tools.Media.Audio.Language,
				MemoryLimitMB: cfg.Tools.Media.Audio.MemoryLimitMB,
			},
			PDF: bot.PDFConfig{
				MaxPages: cfg.Tools.Media.PDF.MaxPages,
			},
		},
	}
	toolRegistry := bot.InitializeTools(toolConfig)
	toolExecutor := bot.NewToolExecutor(toolRegistry)

	// Initialize Bot Handler
	handler := bot.NewHandler(
		llmClient,
		embeddingClient,
		memoryStore,
		bot.HandlerConfig{
			MessageProcessingDelay:     cfg.Delays.MessageProcessing,
			FactAgingDays:              cfg.MemorySettings.FactAgingDays,
			FactSummarizationThreshold: cfg.MemorySettings.FactSummarizationThreshold,
			MaintenanceIntervalHours:   cfg.MemorySettings.MaintenanceIntervalHours,
		},
	)
	handler.SetToolExecutor(toolExecutor)

	// Create Discord Session
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}
	// The core requirement: Masquerade as an Android client
	dg.Identify.Properties.Browser = "Discord Android"
	dg.Identify.Properties.Device = "Discord Android" // Consistency

	// Register handlers
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handler.HandleMessage(&bot.DiscordSession{s}, m)
	})

	// Open connection
	dg.Open()
	defer dg.Close()

	// Set bot ID so handler can ignore its own messages
	if dg.State != nil && dg.State.User != nil {
		handler.SetBotID(dg.State.User.ID)
	}

	log.Println("Marin is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
}
