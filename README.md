# 🎀 Marin AI

> *A Discord companion bot that never forgets you*

Marin is an AI-powered Discord bot featuring **long-term memory**, **semantic search**, and a unique personality inspired by Marin Kitagawa. She remembers your conversations, learns facts about you, and even reaches out when she's feeling lonely.

![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/License-Curse%20of%20Knowledge-maroon?style=flat-square)
![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat-square&logo=docker)

---

## ✨ Features

### 🧠 Intelligent Memory System
- **Semantic Vector Memory** — Stores conversation embeddings for context-aware recall
- **User Profile Facts** — Automatically extracts and maintains persistent facts about users (name, preferences, location, etc.)
- **Smart Deduplication** — Prevents storing redundant information using cosine similarity
- **Memory Maintenance** — Periodic cleanup and summarization of aging facts

### 💬 Natural Conversations
- **Multi-Model Failover** — Seamlessly switches between Cerebras-hosted models (Llama 3.3 70B, Qwen 3 235B, etc.) on API failures
- **Discord-Native Style** — Casual texting style with custom emoji support
- **Mood-Aware Reactions** — Uses zero-shot classification to add contextual emoji reactions
- **Message Chunking** — Handles Discord's 2000 character limit gracefully

### ⏰ Proactive Behaviors
- **Loneliness System** — Sends DMs to inactive users (Duolingo-style, won't spam if unanswered)
- **Reminders** — Extracts and schedules event reminders from conversations
- **Typing Indicators** — Natural typing simulation with configurable delays

### 🖼️ Image Understanding
- **Vision-Enabled** — Sees and reacts to images users send (powered by Gemini Latest Flash Lite)
- **Natural Descriptions** — Images are described contextually for the main LLM
- **NSFW Detection** — Gracefully handles blocked/flagged content

### 🛡️ Privacy & Control
- **`/reset` Command** — Users can permanently delete all their data
- **Per-User Isolation** — Each user's memories are stored separately
- **No Third-Party Data Sharing** — All data stays in your SurrealDB instance

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Discord Gateway                         │
└─────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                          Bot Handler                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │  Message    │  │  Slash      │  │   Background Tasks      │  │
│  │  Handler    │  │  Commands   │  │  • Loneliness Check     │  │
│  │             │  │  • /reset   │  │  • Reminder Polling     │  │
│  └─────────────┘  └─────────────┘  │  • Memory Maintenance   │  │
│                                    └─────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
        │                     │                      │
        ▼                     ▼                      ▼
┌───────────────┐  ┌─────────────────┐  ┌─────────────────────────┐
│   Cerebras    │  │   Embedding     │  │      Classifier         │
│   LLM API     │  │   API           │  │   (HuggingFace NLI)     │
│               │  │                 │  │                         │
│ • llama-3.3   │  │ Vector          │  │  Zero-shot mood/        │
│ • qwen-3-235b │  │ generation      │  │  intent classification  │
│ • + fallbacks │  │ for semantic    │  │  for emoji reactions    │
└───────────────┘  │ search          │  └─────────────────────────┘
        │          └─────────────────┘              │
        │                   │                       │
        │                   ▼                       │
        │     ┌─────────────────────────────┐       │
        │     │        SurrealDB            │       │
        │     │                             │       │
        │     │  • User profiles & facts    │       │
        │     │  • Vector memories          │       │
        │     │  • Recent message cache     │       │
        │     │  • Reminders                │       │
        │     │  • Emoji cache              │       │
        │     │  • Pending DM tracking      │       │
        │     └─────────────────────────────┘       │
        │                                           │
        └─────────────────┬─────────────────────────┘
                          ▼
              ┌─────────────────────────────┐
              │     Gemini Vision API       │
              │   (Image Understanding)     │
              │                             │
              │  • Gemini 2.0 Flash Lite    │
              │  • Image → Description      │
              │  • NSFW detection           │
              └─────────────────────────────┘
```

---

## 📦 Project Structure

```
marinai/
├── main.go                 # Application entrypoint
├── config.yml              # Runtime configuration
├── pkg/
│   ├── bot/                # Discord bot logic
│   │   ├── handler.go          # Main message handler
│   │   ├── system_prompt.go    # Marin's personality prompt
│   │   ├── memory_processing.go# Fact extraction from conversations
│   │   ├── loneliness.go       # Proactive DM system
│   │   ├── slash_commands.go   # Discord slash commands
│   │   ├── reactions.go        # Emoji reaction logic
│   │   ├── reminders.go        # Reminder polling
│   │   └── ...
│   ├── cerebras/           # Cerebras API client with model failover
│   ├── classifier/         # HuggingFace zero-shot classifier
│   ├── embedding/          # Text embedding API client
│   ├── vision/             # Gemini Vision API for image understanding
│   ├── memory/             # Memory store interface & implementations
│   │   ├── store.go            # Store interface definition
│   │   ├── surreal_store.go    # SurrealDB implementation
│   │   └── memory_management.go# Cleanup & summarization
│   ├── surreal/            # SurrealDB WebSocket client
│   └── config/             # YAML config loading
├── .github/workflows/      # CI/CD (tests & releases)
├── Dockerfile              # Multi-stage production build
└── docker-compose.yml      # Container orchestration
```

---

## 🚀 Getting Started

### Prerequisites

- **Go 1.24+** (or Docker)
- **SurrealDB** instance (local or cloud)
- API keys for:
  - Discord Bot Token
  - Cerebras API
  - Embedding API (e.g., your own or a service)
  - HuggingFace API (for classifier)
  - Gemini API (optional, for image understanding)

### Configuration

1. **Clone the repository**
   ```bash
   git clone https://github.com/yourusername/marinai.git
   cd marinai
   ```

2. **Create environment file**
   ```bash
   cp example.env .env
   ```

3. **Fill in your secrets** in `.env`:
   ```env
   DISCORD_TOKEN=your_discord_bot_token
   CEREBRAS_API_KEY=your_cerebras_key
   EMBEDDING_API_KEY=your_embedding_key
   EMBEDDING_API_URL=https://your-embedding-endpoint/embed
   HF_API_KEY=your_huggingface_key
   SURREAL_DB_HOST=your-surrealdb-host.com
   SURREAL_DB_USER=root
   SURREAL_DB_PASS=your_password
   SURREAL_DB_NAMESPACE=marin    # optional, defaults to 'marin'
   SURREAL_DB_DATABASE=memory    # optional, defaults to 'memory'
   GEMINI_API_KEY=your_gemini_key  # optional, enables image understanding
   ```

4. **Adjust `config.yml`** if needed:
   ```yaml
   model_settings:
     temperature: 1
     top_p: 1
   delays:
     message_processing: 1.5  # seconds of typing simulation
   memory:
     fact_aging_days: 7
     fact_summarization_threshold: 20
     maintenance_interval_hours: 24
   ```

### Running Locally

```bash
# Install dependencies
go mod download

# Run the bot
go run main.go
```

### Running with Docker

```bash
# Build and start
docker compose up -d

# View logs
docker compose logs -f marinai
```

---

## 🧪 Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./pkg/memory/...
go test ./pkg/bot/...
```

---

## 📡 API Integrations

| Service | Purpose | Fallback |
|---------|---------|----------|
| **Cerebras** | LLM chat completions | Auto-cycles through 6 models |
| **Gemini** | Image understanding | Optional (graceful disable) |
| **Embedding API** | Text → vector embeddings | Configurable endpoint |
| **HuggingFace** | Zero-shot classification | Cached per-message |
| **SurrealDB** | Persistent storage | Required (no fallback) |

### Cerebras Model Priority

The bot automatically tries models in this order:
1. `llama-3.3-70b` (64k context)
2. `zai-glm-4.6` (64k context)
3. `llama3.1-8b` (8k context)
4. `qwen-3-235b-a22b-instruct-2507` (64k context)
5. `qwen-3-32b` (64k context)
6. `gpt-oss-120b` (64k context)

---

## 🎮 Discord Commands

| Command | Description |
|---------|-------------|
| `/reset` | Permanently delete all your conversation history and memories |
| `/stats` | See what Marin remembers about you (your stored facts) |
| `/mood` | Check Marin's current mood state |

---

## 🎭 Mood System

Marin has 7 different moods that change based on time, activity level, and day of week:

| Mood | Emoji | Trigger | Behavior |
|------|-------|---------|----------|
| **HAPPY** | 😊 | Default state | Bubbly and friendly |
| **HYPER** | ⚡ | High message rate (20+/5min) | Excited, uses caps, exclamation marks |
| **SLEEPY** | 😴 | Late night (11pm-7am) | Drowsy, uses lowercase, typos |
| **BORED** | 😐 | Low activity during daytime | Listless, may change subjects |
| **FLIRTY** | 💋 | Weekend evenings | Extra teasing and playful |
| **FOCUSED** | 🎯 | Weekday work hours | Brief and to-the-point |
| **NOSTALGIC** | 🌸 | Sunday afternoons | References old memories, wistful |

Mood also affects:
- **Typing speed** — Hyper types fast, Sleepy types slow
- **Reaction frequency** — More reactive when Hyper/Flirty, less when Sleepy/Focused
- **Response style** — Each mood has unique LLM instructions

---

## 🔧 How Memory Works

1. **Conversation happens** → Message stored in recent messages cache
2. **Heuristic filter** → Checks for self-referential keywords ("I am", "my name", etc.)
3. **LLM analysis** → Extracts facts and checks for contradictions with existing profile
4. **Delta application** → Adds new facts, removes contradicted ones
5. **Embedding generation** → Stores vector for semantic search
6. **Semantic retrieval** → On future queries, finds relevant past context

### Example Flow

```
User: "I just moved to Tokyo for my new job at Sony!"

→ Heuristic triggers: "I", "my"
→ LLM extracts: { add: ["Lives in Tokyo", "Works at Sony"], remove: ["Lives in Seattle"] }
→ Profile updated
→ Next conversation: "How's Japan treating you?" uses Tokyo context
```

---

## 🤝 Contributing

Contributions are welcome! Please ensure:
- Tests pass: `go test ./...`
- Code is formatted: `go fmt ./...`
- No new linting issues: `go vet ./...`

---

## 📜 License

This project is licensed under **The Curse of Knowledge License v1.0**.

> By reading any portion of this license, you have already violated its primary condition.

See [LICENSE](LICENSE) for the full (self-referential) text.

---

## 🙏 Acknowledgments

- Built with [discordgo](https://github.com/bwmarrin/discordgo)
- Powered by [Cerebras](https://cerebras.ai/) for lightning-fast inference
- Data stored in [SurrealDB](https://surrealdb.com/)
- Inspired by [My Dress-Up Darling](https://en.wikipedia.org/wiki/My_Dress-Up_Darling)