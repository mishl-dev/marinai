<p align="center">
  <img src="https://raw.githubusercontent.com/mishl-dev/marinai/main/.github/assets/banner.png" alt="Marin AI Banner" width="100%"/>
</p>

<h1 align="center">✨ Marin AI ✨</h1>

<p align="center">
  <strong>An AI-powered Discord companion inspired by Marin Kitagawa</strong>
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
A sophisticated relationship system inspired by dating sims with **100,000 XP** max progression and **10 relationship levels**:

| Level | XP Range | Description |
|-------|----------|-------------|
| 👋 Stranger | 0 - 4,999 | Just met |
| 👀 Familiar Face | 5,000 - 9,999 | Seen around |
| 🙂 Acquaintance | 10,000 - 19,999 | Starting to know you |
| 😊 Casual Friend | 20,000 - 34,999 | Hang out sometimes |
| 😄 Friend | 35,000 - 49,999 | Actual friends |
| 🤗 Good Friend | 50,000 - 64,999 | Genuinely close |
| 💕 Close Friend | 65,000 - 79,999 | Deep trust |
| 💗 Best Friend | 80,000 - 89,999 | No barriers |
| 💖 Soulmate | 90,000 - 97,499 | Deep connection |
| ❤️‍🔥 Special Someone | 97,500 - 100,000 | In love |

#### 🔥 Daily Streaks
- Interact every day to build a streak (up to **2x XP bonus** at 30+ days!)
- Missing a day resets your streak with a small penalty
- Streak displayed in `/affection` command

#### 🎯 Mood Multipliers
Your affection gains are affected by Marin's current mood:
- **Flirty** (1.5x) — Compliments and flirting worth more
- **Hyper** (1.2x) — Everything feels more meaningful
- **Playful** (1.3x) — Teasing and jokes rewarded
- **Bored** (0.6x) — Harder to impress

#### 🏆 Milestone Events
When you level up, Marin sends you a special DM with:
- A heartfelt message acknowledging your bond
- Personal secrets she only shares with close friends

#### 💚 Jealousy System
If you're active in a server but haven't talked to Marin in 3+ days:
- She notices and gets a little jealous
- You might receive a passive-aggressive comment
- Small affection penalty until you make it up to her

#### ✨ Random Events
5% chance per message of triggering a "heart moment":
- Bonus affection from random thoughts Marin has about you
- Messages like *"wait... my heart just did a thing 💕"*

#### 🎁 Hidden Bonuses
- **Shared interests** — Talking about cosplay, anime, etc. gives extra XP
- **Late night chats** (11 PM - 4 AM) — More intimate conversations
- **DM conversations** — Worth more than public channels
- **Anniversary tracking** — Special messages on milestones (7 days, 30 days, 1 year, etc.)

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
- If you haven't talked to Marin in 1+ day, she might DM you
- "hey... haven't heard from you in a while..."
- Won't spam — only one pending DM at a time
- Responding to her DM gives bonus affection!

### 🧠 Agency System (NEW!)
Marin has her own **internal state** that makes her feel alive:

#### 💭 Proactive Thoughts
- Marin thinks about close friends and reaches out unprompted
- Higher affection = higher chance of random messages
- Messages reference what she's currently doing/thinking

#### 📓 Personal Journal
- Marin has her own activities, projects, and thoughts
- **Current Activity**: "working on a cosplay", "watching anime", etc.
- **Current Project**: "Miku cosplay", "a bunny girl costume", etc.
- **On Her Mind**: Random thoughts that influence conversations
- These shift over time (every 30 minutes)

#### 🎭 State-Aware Responses
- Marin references her current activity in conversations
- Her mood is influenced by how recent interactions went
- Creates a sense of continuity and presence

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