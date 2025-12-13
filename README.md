# MarinAI

MarinAI is a sophisticated Discord bot built with Go, designed to bring the personality of Marin Kitagawa to life. It leverages advanced AI technologies for natural conversation, long-term memory, and image understanding, creating a persistent and engaging companion.

## Features

*   **Conversational AI:** Engages in natural, character-consistent chat using the Cerebras API (Llama 3.1).
*   **Long-term Memory (RAG):** Remembers user facts, preferences, and past conversations using vector search stored in SurrealDB.
*   **Image Understanding:** Can view and discuss images sent by users using Gemini Flash Lite.
*   **Smart Reminders:** Intelligently parses conversation to set reminders (e.g., "remind me to buy milk tomorrow").
*   **Mood System:** Changes mood (Happy, Hyper, Sleepy, etc.) based on interactions and time of day, affecting responses.
*   **Loneliness System:** Proactively messages users if ignored for too long, maintaining a sense of agency.
*   **Task Refusal:** Refuses "boring" tasks (like writing essays) to maintain character integrity.
*   **Slash Commands:**
    *   `/reset`: Wipes your memory profile.
    *   `/stats`: Displays what Marin remembers about you.
    *   `/mood`: Shows Marin's current mood.
    *   `/affection`: Shows your relationship status with Marin.

## Tech Stack

*   **Language:** Go (Golang)
*   **Inference:** Cerebras API (Llama 3.1 70b)
*   **Memory & Database:** SurrealDB
    *   Vector Storage (Embeddings)
    *   User Profiles
    *   Reminders
*   **Image Analysis:** Google Gemini Flash Lite
*   **Embeddings:** Custom Embedding API
*   **Discord Library:** DiscordGo

## Prerequisites

*   **Docker & Docker Compose** (Recommended)
*   Or **Go 1.22+** and a running **SurrealDB** instance.

## Configuration

1.  Clone the repository:
    ```bash
    git clone https://github.com/yourusername/marinai.git
    cd marinai
    ```

2.  Create a `.env` file based on `example.env`:
    ```bash
    cp example.env .env
    ```

3.  Fill in your API keys and credentials in `.env`:
    ```env
    DISCORD_TOKEN=your_discord_bot_token
    CEREBRAS_API_KEY=your_cerebras_key
    EMBEDDING_API_KEY=your_embedding_key
    GEMINI_API_KEY=your_gemini_key (optional, for image support)
    SURREAL_DB_HOST=ws://surreal:8000/rpc
    SURREAL_DB_USER=root
    SURREAL_DB_PASS=root
    ```

4.  (Optional) Customize `config.yml` to tweak model settings, delays, or memory behavior.

## Usage

### Running with Docker (Recommended)

```bash
docker-compose up -d --build
```

This will start the bot and a SurrealDB instance as defined in `docker-compose.yml`.

### Running Locally

Ensure SurrealDB is running and accessible.

```bash
go run .
```

## Architecture Overview

*   **`main.go`**: Entry point. Initializes clients, connects to SurrealDB and Discord, and starts the bot.
*   **`pkg/bot/`**: Core bot logic.
    *   `handler.go`: Central event handler, manages message flow and dispatching.
    *   `memory_processing.go`: Logic for extracting facts and reminders from conversation.
    *   `reminders.go`: Reminder scheduling and processing.
    *   `slash_commands.go`: Definitions and handlers for slash commands.
*   **`pkg/memory/`**: Database abstraction layer (SurrealDB).
*   **`pkg/cerebras/`**: Client for the Cerebras inference API.
*   **`pkg/gemini/`**: Client for Google Gemini (image analysis).

## Development

Run tests:
```bash
go test ./...
```
