<p align="center">
  <img src="https://raw.githubusercontent.com/mishl-dev/marinai/main/.github/assets/banner.png" alt="Marin AI Banner" width="100%"/>
</p>

<h1 align="center">✨ Marin AI ✨</h1>

<p align="center">
  <strong>An AI-powered Discord companion inspired by Marin Kitagawa</strong>
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#commands">Commands</a> •
  <a href="#architecture">Architecture</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go" alt="Go Version"/>
  <img src="https://img.shields.io/badge/Discord-Bot-5865F2?style=flat-square&logo=discord&logoColor=white" alt="Discord Bot"/>
  <img src="https://img.shields.io/badge/SurrealDB-Powered-FF00A0?style=flat-square" alt="SurrealDB"/>
  <img src="https://img.shields.io/badge/Cerebras-AI-orange?style=flat-square" alt="Cerebras AI"/>
</p>

---

## 🌸 About

**Marin AI** is a Discord bot that embodies the personality of Marin Kitagawa — the bubbly, passionate cosplayer from *My Dress-Up Darling*. She's not just another chatbot; she **remembers** you, develops **relationships** over time, and has her own **moods** that affect how she interacts with you.

Built with love using Go, Cerebras AI for ultra-fast inference, and SurrealDB for persistent memory.

---

## ✨ Features

### 🧠 Persistent Memory
- **Vector-based memory** — Marin remembers conversations using semantic embeddings
- **Fact extraction** — She learns and stores facts about users (interests, preferences, etc.)
- **Memory maintenance** — Automatic aging and summarization of old memories
- **Duplicate detection** — Smart deduplication prevents redundant memories

### 💕 Affection System
A sophisticated relationship system inspired by dating sims:

| Level | XP Range | Description |
|-------|----------|-------------|
| 👋 Stranger | 0 - 999 | Just met |
| 🙂 Acquaintance | 1,000 - 2,499 | Starting to recognize you |
| 😊 Friend | 2,500 - 4,999 | Comfortable and casual |
| 💕 Close Friend | 5,000 - 7,499 | Sharing personal things |
| 💗 Best Friend | 7,500 - 8,999 | No barriers |
| ❤️ Special Someone | 9,000 - 10,000 | Maximum affection |

**How it works:**
- Affection increases through positive interactions (compliments, long conversations, being supportive)
- Affection decreases from negative behaviors (being rude, dismissive, or ignoring her)
- Natural decay over time if you don't interact (but closer relationships decay slower!)
- Marin's personality adapts based on your relationship level

### 🎭 Dynamic Moods
Marin has different moods that cycle naturally and affect her responses:

- ✨ **Hyper** — Extra energetic and excitable
- 😴 **Sleepy** — A bit drowsy, shorter responses
- 😏 **Flirty** — More teasing and playful
- 🌸 **Nostalgic** — Reflective and wistful
- 🎯 **Focused** — Task-oriented, less distracted
- 😤 **Bored** — Looking for entertainment

### 📸 Image Understanding
- Powered by **Google Gemini** for vision capabilities
- Marin can see and comment on images you send
- Natural reactions to photos and memes

### 😺 Emoji Reactions
- Uses custom guild emojis when available
- Smart reaction matching based on message content
- Personality-appropriate emoji selection

### 💌 Boredom DMs (Duolingo-style)
- If you haven't talked to Marin in 2+ days, she might DM you
- "hey... haven't heard from you in a while..."
- Won't spam — only one pending DM at a time
- Responding to her DM gives bonus affection!

### ⏰ Reminders
- Set reminders that Marin will deliver
- Automatic cleanup of old reminders

---

## 🚀 Installation

### Prerequisites
- Go 1.24 or higher
- SurrealDB instance
- Discord Bot Token
- API Keys for:
  - Cerebras (primary LLM)
  - Embedding API
  - Google Gemini (optional, for vision)

### Quick Start

1. **Clone the repository**
   ```bash
   git clone https://github.com/mishl-dev/marinai.git
   cd marinai
   ```

2. **Copy example environment file**
   ```bash
   cp example.env .env
   ```

3. **Configure your `.env`**
   ```env
   DISCORD_TOKEN=your_discord_bot_token
   CEREBRAS_API_KEY=your_cerebras_key
   EMBEDDING_API_URL=your_embedding_endpoint
   EMBEDDING_API_KEY=your_embedding_key
   SURREAL_DB_HOST=your_surreal_host
   SURREAL_DB_USER=root
   SURREAL_DB_PASS=your_password
   GEMINI_API_KEY=your_gemini_key  # Optional
   ```

4. **Run the bot**
   ```bash
   go run main.go
   ```

### Docker Deployment

```bash
# Build and run with Docker Compose
docker-compose up -d
```

The included `docker-compose.yml` handles environment variables and volume mounting for the config file.

---

## ⚙️ Configuration

### config.yml

```yaml
model_settings:
  temperature: 1      # LLM creativity (0-2)
  top_p: 1           # Nucleus sampling

delays:
  message_processing: 1.5   # Seconds before responding (typing simulation)

memory:
  fact_aging_days: 7               # Days before facts start aging
  fact_summarization_threshold: 20 # Max facts before summarization
  maintenance_interval_hours: 24   # How often to run memory maintenance
```

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DISCORD_TOKEN` | ✅ | Your Discord bot token |
| `CEREBRAS_API_KEY` | ✅ | Cerebras API key for LLM |
| `EMBEDDING_API_URL` | ✅ | Endpoint for text embeddings |
| `EMBEDDING_API_KEY` | ✅ | API key for embeddings |
| `SURREAL_DB_HOST` | ✅ | SurrealDB WebSocket URL |
| `SURREAL_DB_USER` | ✅ | SurrealDB username |
| `SURREAL_DB_PASS` | ✅ | SurrealDB password |
| `SURREAL_DB_NAMESPACE` | ❌ | Namespace (default: `marin`) |
| `SURREAL_DB_DATABASE` | ❌ | Database (default: `memory`) |
| `GEMINI_API_KEY` | ❌ | Google Gemini key for vision |
| `DISCORD_GUILD_ID` | ❌ | Guild ID for faster command updates during dev |

---

## 💬 Commands

### Slash Commands

| Command | Description |
|---------|-------------|
| `/reset` | Permanently delete all your conversation history and memories |
| `/stats` | See what Marin remembers about you |
| `/mood` | Check Marin's current mood |
| `/affection` | Check your relationship status with Marin |

### Interacting with Marin

Marin responds when:
- **Mentioned** (`@Marin hey!`)
- **Replied to** (reply to any of her messages)
- **Random chance** (~30% in active channels)
- **DMs** (always responds in direct messages)

---

## 🏗️ Architecture

```
marinai/
├── main.go                 # Entry point, initialization
├── config.yml              # Bot configuration
├── Dockerfile              # Multi-stage build
├── docker-compose.yml      # Container orchestration
└── pkg/
    ├── bot/                # Core bot logic
    │   ├── handler.go      # Message handling
    │   ├── affection.go    # Relationship system
    │   ├── mood.go         # Mood system
    │   ├── memory_*.go     # Memory processing
    │   ├── slash_commands.go
    │   └── system_prompt.go
    ├── cerebras/           # Cerebras LLM client
    ├── gemini/             # Google Gemini adapter
    ├── embedding/          # Text embedding client
    ├── memory/             # Memory store interface
    └── surreal/            # SurrealDB client
```

### Tech Stack

- **Language**: Go 1.24
- **Discord Library**: [discordgo](https://github.com/bwmarrin/discordgo)
- **Database**: [SurrealDB](https://surrealdb.com/) — Vector search + document storage
- **LLM**: [Cerebras](https://cerebras.ai/) — Ultra-fast inference
- **Vision**: [Google Gemini](https://ai.google.dev/) — Image understanding
- **Caching**: In-memory LRU cache for embeddings

---

## 🔧 Development

### Running Tests

```bash
go test ./...
```

### Project Structure

| Package | Purpose |
|---------|---------|
| `pkg/bot` | Discord event handlers, personality logic |
| `pkg/cerebras` | LLM API client |
| `pkg/gemini` | Vision API adapter |
| `pkg/embedding` | Text embedding with caching |
| `pkg/memory` | Memory store abstraction |
| `pkg/surreal` | SurrealDB client wrapper |

---

## 📜 License

This project uses **The Curse of Knowledge License** — a satirical license where reading it revokes all rights. In practice: do whatever you want, just don't be weird about it.

---

## 🙏 Acknowledgments

- **Marin Kitagawa** — The character from *My Dress-Up Darling* by Shinichi Fukuda
- **Cerebras** — For providing ultra-fast LLM inference
- **SurrealDB** — For the excellent database with built-in vector search