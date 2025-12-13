# 🌸 Marin AI - Discord Bot

A sophisticated Discord bot that embodies Marin Kitagawa from *My Dress-Up Darling* - complete with dynamic personality, memory systems, and relationship mechanics.

## ✨ Features

### 🧠 Advanced Memory System
- **Vector-based RAG**: Semantic search through conversation history using embeddings
- **Fact extraction**: Automatically learns and remembers personal information about users
- **Memory aging**: Facts older than 7 days are archived to vector storage
- **Automatic summarization**: Consolidates profile facts when threshold is reached
- **Rolling context**: Maintains last 15 messages for coherent conversations

### 💕 Relationship System
- **Affection tracking**: 6-tier progression from Stranger (0) to Special Someone (10,000 XP)
- **Behavioral analysis**: LLM-powered sentiment detection adjusts affection based on interactions
- **Natural decay**: Affection decays over time based on relationship level (prevents grinding)
- **Contextual bonuses**: DMs, mentions, compliments, vulnerability all increase affection
- **Dynamic personality**: Marin's responses adapt to your relationship level

### 🎭 Dynamic Mood System
- **7 distinct moods**: HAPPY, HYPER, SLEEPY, BORED, FLIRTY, FOCUSED, NOSTALGIC
- **Time-aware**: Changes based on Tokyo timezone and time of day
- **Activity-based**: Responds to message rate and server activity
- **Status integration**: Mood affects Discord status, typing speed, and reaction likelihood

### 🤖 Intelligence Features
- **Multi-model fallback**: Cycles through Cerebras models (Llama 3.3 70B → ZaI GLM → others)
- **Task detection**: Refuses long writing tasks with in-character responses
- **Image understanding**: Gemini Flash Lite for vision capabilities
- **Smart reactions**: Classifies messages and reacts with appropriate emojis
- **Loneliness system**: Duolingo-style DMs when inactive (won't spam if ignored)

### ⏰ Reminder System
- **Natural language**: Extracts reminders from conversation context
- **Smart delivery**: Sends contextual, in-character reminder messages
- **Automatic cleanup**: Removes old reminders after 24 hours

### 🎨 Discord Features
- **Slash commands**: `/stats`, `/reset`, `/mood`, `/affection`
- **Custom emoji support**: Filters and uses server emojis relevant to Marin's interests
- **Typing simulation**: Realistic typing delays based on message length and mood
- **Split messages**: Long replies broken into natural chunks

## 🏗️ Architecture

```
┌─────────────────┐
│  Discord Bot    │
│  (DiscordGo)    │
└────────┬────────┘
         │
    ┌────▼─────────────────────────────────┐
    │         Handler Layer                │
    │  • Message routing                   │
    │  • Mood management                   │
    │  • Affection calculations            │
    └────┬─────────────────────────────────┘
         │
    ┌────▼──────────────┬──────────────────┬──────────────┐
    │                   │                  │              │
┌───▼──────┐    ┌──────▼──────┐    ┌─────▼─────┐  ┌────▼─────┐
│ Cerebras │    │   Gemini    │    │ Embedding │  │ SurrealDB│
│   LLM    │    │Flash Lite   │    │   API     │  │  Memory  │
└──────────┘    └─────────────┘    └───────────┘  └──────────┘
```

### Core Components

**`pkg/bot/handler.go`** - Central message processing and orchestration
**`pkg/memory/surreal_store.go`** - SurrealDB interface for persistent storage
**`pkg/cerebras/client.go`** - Multi-model LLM client with automatic fallback
**`pkg/gemini/client.go`** - Vision and fast text classification
**`pkg/embedding/client.go`** - Vector embedding generation with LRU cache

## 🚀 Quick Start

### Prerequisites
- Go 1.24+
- SurrealDB instance
- API keys for:
  - Discord Bot Token
  - Cerebras API
  - Embedding API
  - Gemini API (optional, for vision)

### Installation

1. **Clone the repository**
```bash
git clone https://github.com/yourusername/marinai.git
cd marinai
```

2. **Configure environment**
```bash
cp example.env .env
# Edit .env with your API keys
```

3. **Run with Docker Compose**
```bash
docker-compose up -d
```

**Or build manually:**
```bash
go mod download
go build -o marinai ./main.go
./marinai
```

## ⚙️ Configuration

### Environment Variables

```env
# Required
DISCORD_TOKEN=your_discord_bot_token
CEREBRAS_API_KEY=your_cerebras_key
EMBEDDING_API_KEY=your_embedding_key
SURREAL_DB_HOST=wss://your-db.example.com/rpc
SURREAL_DB_USER=your_user
SURREAL_DB_PASS=your_password

# Optional
GEMINI_API_KEY=your_gemini_key
EMBEDDING_API_URL=https://vector.mishl.dev/embed
SURREAL_DB_NAMESPACE=marin
SURREAL_DB_DATABASE=memory
DISCORD_GUILD_ID=your_guild_for_testing
```

### config.yml

```yaml
model_settings:
  temperature: 1      # LLM creativity (0-2)
  top_p: 1           # Nucleus sampling

delays:
  message_processing: 1.5  # Seconds between multi-part messages

memory:
  fact_aging_days: 7                    # Days before facts archive
  fact_summarization_threshold: 20      # Facts before summarization
  maintenance_interval_hours: 24        # Memory maintenance frequency
```

## 📊 Database Schema

SurrealDB tables used by the bot:

- **`memories`** - Vector embeddings for semantic search (2048-dim, cosine similarity)
- **`user_profiles`** - User facts, affection, last interaction timestamps
- **`recent_messages`** - Rolling 15-message context window
- **`reminders`** - Scheduled user reminders
- **`guild_cache`** - Cached custom emoji lists
- **`bot_state`** - Global state (current mood, etc.)
- **`pending_dm`** - Tracks unanswered loneliness DMs

## 🎮 Usage Examples

### Basic Interaction
```
User: @Marin what do you think about cosplay?
Marin: cosplay is literally my life??? like i cant imagine doing anything else
       working on a shizuku-tan costume rn and its SO GOOD
```

### Affection System
```
/affection
💕 Close Friend
████████░░
5,234 / 7,500 XP to next level
```

### Memory Extraction
```
User: I just started learning Japanese!
Marin: oh thats sick!! you gonna watch anime without subs?

# Bot automatically saves: "User is learning Japanese"
```

### Mood System
```
/mood
😴 SLEEPY
so tired... need sleep

*yawns*
```

## 🧪 Testing

```bash
# Run all tests
go test -v ./...

# Test specific package
go test -v ./pkg/bot

# Test with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## 🚢 Deployment

### Docker (Recommended)
```bash
docker-compose up -d
```

### GitHub Actions
Auto-releases on push to `master`:
- Runs tests
- Builds binary
- Creates Docker image
- Publishes to GitHub Container Registry

```bash
docker pull ghcr.io/yourusername/marinai:latest
```

## 🤝 Contributing

Contributions welcome! Areas for improvement:

- [ ] Multi-language support
- [ ] Voice channel integration
- [ ] Image generation for "selfies"
- [ ] Costume/outfit system (see `ideas.md`)
- [ ] Enhanced proactive behaviors

## 📜 License

**THE CURSE OF KNOWLEDGE LICENSE v1.0** - See `LICENSE` for the paradoxical details.

> *Warning: Reading the license revokes your rights to use this software. Proceed with caution.*

## 🙏 Acknowledgments

- **Character Design**: Marin Kitagawa from *Sono Bisque Doll wa Koi wo Suru* (©Shinichi Fukuda)
- **LLM**: Powered by [Cerebras](https://cerebras.ai/) and [Google Gemini](https://ai.google.dev/)
- **Discord Library**: [DiscordGo](https://github.com/bwmarrin/discordgo)
- **Database**: [SurrealDB](https://surrealdb.com/)
