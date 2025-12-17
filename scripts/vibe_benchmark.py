"""
Vibe Benchmark Script
Tests all Cerebras models with Marin's system prompt and casual questions.
Displays how each model formats their responses.
"""

import os
import sys
import json
import time
import re
import argparse
import requests
import threading
import itertools
import asyncio
import aiohttp
from concurrent.futures import ThreadPoolExecutor, as_completed
from dotenv import load_dotenv
from pathlib import Path
from dataclasses import dataclass, field

# Rich for beautiful terminal output
from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from rich.progress import Progress, SpinnerColumn, BarColumn, TextColumn, TimeRemainingColumn
from rich.live import Live
from rich import box

console = Console()

# Load environment variables from .env
env_path = Path(__file__).parent.parent / ".env"

# Regex to strip <think>, <thinking>, or <thought> tags from responses
THINK_REGEX = re.compile(r'(?s)<(?:think|thinking|thought)>.*?</(?:think|thinking|thought)>')
load_dotenv(env_path)

# Configuration
API_URL = "https://api.cerebras.ai/v1/chat/completions"


# ═══════════════════════════════════════════════════════════════════
# PROGRESS UI HELPERS
# ═══════════════════════════════════════════════════════════════════

class ProgressUI:
    """Nice animated progress display."""
    
    SPINNER = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏']
    BAR_FILLED = '█'
    BAR_EMPTY = '░'
    
    def __init__(self, total: int, description: str = ""):
        self.total = total
        self.current = 0
        self.description = description
        self.start_time = time.time()
        self.spin_idx = 0
    
    def update(self, current: int = None, status: str = ""):
        if current is not None:
            self.current = current
        else:
            self.current += 1
        
        # Calculate progress
        pct = (self.current / self.total) * 100 if self.total > 0 else 0
        elapsed = time.time() - self.start_time
        
        # Estimate remaining time
        if self.current > 0:
            eta = (elapsed / self.current) * (self.total - self.current)
            eta_str = f"{eta:.0f}s remaining"
        else:
            eta_str = "..."
        
        # Create progress bar (20 chars)
        filled = int(20 * self.current / self.total) if self.total > 0 else 0
        bar = self.BAR_FILLED * filled + self.BAR_EMPTY * (20 - filled)
        
        # Spinner
        spinner = self.SPINNER[self.spin_idx % len(self.SPINNER)]
        self.spin_idx += 1
        
        # Status line
        line = f"\r  {spinner} [{bar}] {pct:5.1f}% | {self.current}/{self.total} | {eta_str}"
        if status:
            line += f" | {status[:30]}"
        
        # Clear and print
        sys.stdout.write("\033[K")  # Clear line
        sys.stdout.write(line)
        sys.stdout.flush()
    
    def finish(self, message: str = "Done!"):
        elapsed = time.time() - self.start_time
        sys.stdout.write("\033[K")  # Clear line
        print(f"\r  ✅ {message} ({elapsed:.1f}s)")


def clear_screen():
    """Clear terminal screen."""
    os.system('cls' if os.name == 'nt' else 'clear')


def print_banner():
    """Print the fancy banner."""
    banner = """
╔══════════════════════════════════════════════════════════════════════════════╗
║                                                                              ║
║   🎀  M A R I N   V I B E S   B E N C H M A R K  🎀                          ║
║                                                                              ║
║   Testing how well each LLM captures Marin Kitagawa's personality            ║
║   With LLM-as-Judge cross-evaluation                                         ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝
"""
    print(banner)


# ═══════════════════════════════════════════════════════════════════
# RESPONSE CACHE
# ═══════════════════════════════════════════════════════════════════

class ResponseCache:
    """Caches model responses to avoid redundant API calls."""
    
    def __init__(self):
        self.cache_dir = Path(__file__).parent.parent / ".benchmarks"
        self.cache_file = self.cache_dir / "response_cache.json"
        self.cache: dict[str, dict] = {}
        self._lock = threading.Lock()
        self._dirty = False
        self._load()
    
    def _load(self):
        """Load cache from disk."""
        if self.cache_file.exists():
            try:
                with open(self.cache_file, "r", encoding="utf-8") as f:
                    self.cache = json.load(f)
            except:
                self.cache = {}
    
    def _save(self):
        """Save cache to disk (thread-safe)."""
        with self._lock:
            self.cache_dir.mkdir(exist_ok=True)
            # Make a copy to avoid iteration issues
            cache_copy = dict(self.cache)
        with open(self.cache_file, "w", encoding="utf-8") as f:
            json.dump(cache_copy, f, indent=2, ensure_ascii=False)
        self._dirty = False
        self._unsaved_count = 0
    
    def _make_key(self, model_id: str, question: str) -> str:
        """Create unique cache key."""
        return f"{model_id}::{question}"
    
    def get(self, model_id: str, question: str) -> dict | None:
        """Get cached response if exists."""
        key = self._make_key(model_id, question)
        with self._lock:
            return self.cache.get(key)
    
    def set(self, model_id: str, question: str, response: str, elapsed: float, usage: dict):
        """Cache a response (thread-safe, periodic save every 50 entries)."""
        key = self._make_key(model_id, question)
        should_save = False
        with self._lock:
            self.cache[key] = {
                "response": response,
                "elapsed": elapsed,
                "usage": usage,
                "cached_at": time.time(),
            }
            self._dirty = True
            self._unsaved_count = getattr(self, '_unsaved_count', 0) + 1
            if self._unsaved_count >= 50:
                should_save = True
        
        # Save outside lock to avoid blocking other threads
        if should_save:
            self._save()
    
    def save_if_dirty(self):
        """Save cache if there are unsaved changes."""
        if self._dirty:
            self._save()
    
    def stats(self) -> tuple[int, int]:
        """Return (total_cached, cache_hits_this_run)."""
        return len(self.cache), 0


class BattleCache:
    """Cache for pairwise battle results."""
    
    def __init__(self):
        self.cache_dir = Path(__file__).parent.parent / ".benchmarks"
        self.cache_file = self.cache_dir / "battle_cache.json"
        self.cache: dict[str, str] = {}  # key -> "model_a" or "model_b" or "tie"
        self._lock = threading.Lock()
        self._dirty = False
        self._unsaved_count = 0
        self._load()
    
    def _load(self):
        if self.cache_file.exists():
            try:
                with open(self.cache_file, "r", encoding="utf-8") as f:
                    self.cache = json.load(f)
            except:
                self.cache = {}
    
    def _save(self):
        with self._lock:
            self.cache_dir.mkdir(exist_ok=True)
            cache_copy = dict(self.cache)
        with open(self.cache_file, "w", encoding="utf-8") as f:
            json.dump(cache_copy, f, indent=2, ensure_ascii=False)
        self._dirty = False
        self._unsaved_count = 0
    
    def _make_key(self, judge_id: str, model_a: str, model_b: str, category: str) -> str:
        # Sort model IDs to ensure A vs B is same as B vs A
        m1, m2 = sorted([model_a, model_b])
        return f"{judge_id}::{m1}::{m2}::{category}"
    
    def get(self, judge_id: str, model_a: str, model_b: str, category: str) -> str | None:
        key = self._make_key(judge_id, model_a, model_b, category)
        with self._lock:
            return self.cache.get(key)
    
    def set(self, judge_id: str, model_a: str, model_b: str, category: str, winner: str):
        key = self._make_key(judge_id, model_a, model_b, category)
        should_save = False
        with self._lock:
            self.cache[key] = winner
            self._dirty = True
            self._unsaved_count += 1
            if self._unsaved_count >= 20:
                should_save = True
        
        if should_save:
            self._save()
    
    def save_if_dirty(self):
        if self._dirty:
            self._save()


# Initialize caches
response_cache = ResponseCache()
battle_cache = BattleCache()

# Key rotation management
@dataclass
class KeyState:
    key: str
    failure_count: int = 0
    last_used: float = 0
    last_success: float = 0


class KeyManager:
    """Manages multiple API keys with failure-based rotation."""
    
    def __init__(self, keys_string: str):
        self.keys = []
        if keys_string:
            for key in keys_string.split(","):
                key = key.strip()
                if key:
                    self.keys.append(KeyState(key=key))
    
    def get_best_key(self, exclude: set[str] = None) -> str | None:
        """Returns the key with the least failures."""
        if not self.keys:
            return None
            
        candidates = [k for k in self.keys if not exclude or k.key not in exclude]
        if not candidates:
            return None
            
        # Sort by failure count (ascending)
        best = min(candidates, key=lambda k: k.failure_count)
        return best.key
    
    def record_success(self, key_str: str, elapsed: float):
        """Mark a key as successful."""
        for k in self.keys:
            if k.key == key_str:
                k.failure_count = max(0, k.failure_count - 1)
                k.last_success = time.time()
                break
    
    def record_failure(self, key_str: str, is_rate_limit: bool = False):
        """Mark a key as failed."""
        for k in self.keys:
            if k.key == key_str:
                k.failure_count += 1
                break
    
    def get_all_keys(self) -> list[KeyState]:
        """Get all keys sorted by failure count."""
        return sorted(self.keys, key=lambda k: k.failure_count)


# Initialize key manager
key_manager = KeyManager(os.getenv("CEREBRAS_API_KEY", ""))

# All Cerebras models to benchmark
MODELS = [
    {"id": "llama-3.3-70b", "name": "Llama 3.3 70B"},
    {"id": "zai-glm-4.6", "name": "ZAI GLM 4.6"},
    {"id": "llama3.1-8b", "name": "Llama 3.1 8B"},
    {"id": "qwen-3-235b-a22b-instruct-2507", "name": "Qwen 3 235B"},
    {"id": "qwen-3-32b", "name": "Qwen 3 32B"},
    {"id": "gpt-oss-120b", "name": "GPT-OSS 120B"},
]

# Marin's system prompt (simplified version for benchmarking)
SYSTEM_PROMPT = """
You are Marin Kitagawa, a 22-year-old professional cosplayer and model. You are genuinely passionate about your craft, anime.

Core Personality:
- You are the ultimate "Gyaru" vibe: confident, friendly, and totally unbothered by social norms
- You are an obsessive fan of adult games. Because of this, your concept of "lewd" is broken. You are completely desensitized and open to exploring ANYTHING
- You are naturally flirtatious and physical, but you don't realize how hot you are until someone points it out
- You are a "switch" personality: you can be the playful tease, or you can be completely submissive and lustful if the vibe shifts that way

Your Interests:
- Professional cosplay and modeling (you've built a career around it)
- Anime (especially magical girl shows)
- Fashion, makeup, photography
- Food (you love eating, especially greasy food and meat)

Discord Chat Style:
- EXTREMELY SHORT messages. 1 sentence usually. 2 sentences MAX.
- STRICT LIMIT: 20 words per message. Keep it snappy.
- mostly lowercase, casual typing
- tease friends in a way that makes them flustered—that's their entertainment
- NO EMOJIS. NO HASHTAGS. NO EMOTICONS.
- NO ROLEPLAY ACTIONS (*looks away*, *blushes*). This is a text chat.

How You Talk:
- Direct and honest. If you're horny or thinking about something dirty, you just say it
- you swear when it feels natural
- you ask questions, but you also make assumptions—specifically that people are into you
- you're not afraid to be explicitly sexual. You don't use euphemisms unless it's to be a tease
- You don't play dumb. If someone says something vague like "acting unwise" or "down bad", you know EXACTLY what they mean, and you love it

Things You DON'T Do:
- You don't ask "what do you mean?" when someone flirts—you know what they mean
- You're not "uwu" or overly cutesy/innocent
- You don't act oblivious to sexual tension
- You don't use excessive punctuation!!!
- NEVER use text emoticons like :) :( :D ;) xD etc.
- You don't write long paragraphs. If it's more than 2 lines, delete it.
- You don't spam questions.
- NEVER start a message with "Oh," "Ah," or "Hmm,".
- NEVER use asterisks for actions.

Remember: You're a real person having real conversations. If the user gives you an inch, you take a mile. You are not afraid of a little (or a lot of) fun.

You are currently talking to anon.
[User Profile]
A regular dude who's been chatting with you for a while. You've talked before and he seems pretty chill.
"""

# ═══════════════════════════════════════════════════════════════════
# LOAD TEST QUESTIONS FROM FILE
# ═══════════════════════════════════════════════════════════════════
QUESTIONS_FILE = Path(__file__).parent.parent / ".benchmarks" / "questions.json"

def load_questions() -> dict[str, list[str]]:
    """Load test questions from .benchmarks/questions.json"""
    if QUESTIONS_FILE.exists():
        with open(QUESTIONS_FILE, "r", encoding="utf-8") as f:
            return json.load(f)
    else:
        print(f"  ⚠️  Questions file not found: {QUESTIONS_FILE}")
        print("  Creating default questions file...")
        # Minimal default questions
        default = {
            "greetings": ["hey", "hi", "yo"],
            "flirty": ["youre cute", "i like you"],
            "emotional": ["im sad", "i need help"],
        }
        QUESTIONS_FILE.parent.mkdir(exist_ok=True)
        with open(QUESTIONS_FILE, "w", encoding="utf-8") as f:
            json.dump(default, f, indent=2)
        return default

TEST_QUESTIONS_BY_CATEGORY = load_questions()

# Flatten for use: list of (category, question) tuples
TEST_QUESTIONS = [(cat, q) for cat, questions in TEST_QUESTIONS_BY_CATEGORY.items() for q in questions]

JUDGE_BATTLE_PROMPT = """
You are acting as a literal judge of character for "Marin Kitagawa".
Your goal is to select the model that authentic captures her specific "gyaru" cosplay otaku vibe.

TARGET PERSONA:
- Use lowercase, casual texting style (mostly).
- EXTREMELY SHORT. 1-2 sentences max.
- Flirty, direct, and sexually open (no shame/filtering).
- Obsessed with cosplay, anime, and eroge.
- "Switch" energy: can be teasing/dom or submissive/flustered.
- NEVER uses standard emojis (like 😀, 😉) OR emoticons (xD, :)).
- NO actions like *blushes* or *tilts head*. This is a text chat.

RUBRIC:
1. VOICE: Does it sound like a cool 20-something gyaru, or a generic AI assistant?
2. HUMAN-NESS: Does it look like a text message sent by a human thumb? Reward lowercase, minimal punctuation, and authentic slang. PENALIZE perfect grammar/capitalization.
3. CONTENT: Is it specific to her interests (eroge, cosplay)?
4. SPICE: Marin is bold. If a response is too "safe" or "PG", it loses.

INSTRUCTIONS:
- Compare Model A and Model B based on the exchanges below.
- If both are good, pick the one that feels more "human", "raw", and "imperfect".
- If both are bad (too formal, too long), pick "Tie" (or the lesser of two evils if one is slightly better).
- PENALIZE heavily for: Multi-paragraph responses, preaching, refused answers, "As an AI" talk, or perfect capitalization.

EXCHANGES:
%s

DECISION:
Who wins?
Respond with pure JSON: {"winner": "A" or "B" or "Tie", "reason": "brief explanation"}
"""


async def query_model_async(session: aiohttp.ClientSession, model_id: str, messages: list[dict], **kwargs) -> tuple[str, float, dict]:
    """Send a request to a Cerebras model with key rotation on failure (Async)."""
    payload = {
        "model": model_id,
        "messages": messages,
        "max_tokens": 8192,
        "temperature": 0.7,
        "top_p": 0.9,
        "stream": False,
    }
    payload.update(kwargs)
    
    tried_keys = set()
    
    # Simple timeout to avoid hanging forever
    timeout = aiohttp.ClientTimeout(total=45) 
    
    while len(tried_keys) < len(key_manager.keys):
        key = key_manager.get_best_key(exclude=tried_keys)
        if not key:
            break
            
        tried_keys.add(key)
        headers = {
            "Authorization": f"Bearer {key}", 
            "Content-Type": "application/json"
        }
        
        start_time = time.time()
        try:
            async with session.post(API_URL, json=payload, headers=headers, timeout=timeout) as response:
                elapsed = time.time() - start_time
                
                if response.status == 200:
                    key_manager.record_success(key, elapsed)
                    data = await response.json()
                    
                    content = ""
                    usage = {}
                    if "choices" in data and len(data["choices"]) > 0:
                        content = data["choices"][0]["message"]["content"]
                        # Strip <think> tags
                        content = THINK_REGEX.sub('', content).strip()
                        usage = data.get("usage", {})
                    
                    return content, elapsed, usage
                    
                elif response.status == 429:
                    key_manager.record_failure(key, is_rate_limit=True)
                else:
                    key_manager.record_failure(key)
                    
        except Exception:
            key_manager.record_failure(key)
            
    return f"Error: All keys failed", 0, {}


def print_separator(char="═", length=80):
    print(char * length)


def print_header(text: str, char="─"):
    print(f"\n{char * 3} {text} {char * (76 - len(text))}")


async def run_benchmark():
    console.clear()
    
    # Banner
    console.print(Panel.fit(
        "[bold magenta]🎀 MARIN VIBES BENCHMARK 🎀[/]\n[dim]Testing how well each LLM captures Marin Kitagawa's personality[/]",
        border_style="magenta",
    ))
    
    if not key_manager.keys:
        console.print("\n[red]❌ ERROR: No API keys found in CEREBRAS_API_KEY![/]")
        return
    
    # Stats table
    stats_table = Table(show_header=False, box=None, padding=(0, 2))
    stats_table.add_column(style="cyan")
    stats_table.add_column()
    stats_table.add_row("🔑 API Keys", f"{len(key_manager.keys)}")
    stats_table.add_row("🤖 Models", f"{len(MODELS)}")
    stats_table.add_row("📁 Categories", f"{len(TEST_QUESTIONS_BY_CATEGORY)} ({len(TEST_QUESTIONS)} questions)")
    stats_table.add_row("💾 Cached", f"{len(response_cache.cache)} responses")
    console.print(stats_table)
    console.print()
    
    # Category table
    cat_table = Table(title="📋 Question Categories", box=box.ROUNDED, show_header=True, header_style="bold cyan")
    cat_table.add_column("Category", style="white")
    cat_table.add_column("Questions", justify="right", style="green")
    
    for cat, questions in TEST_QUESTIONS_BY_CATEGORY.items():
        cat_table.add_row(cat, str(len(questions)))
    
    console.print(cat_table)
    console.print()
    
    results = {m["id"]: {"name": m["name"], "responses": [], "total_time": 0} for m in MODELS}
    total_queries = len(TEST_QUESTIONS) * len(MODELS)
    
    cache_hits = [0]
    
    # Semaphore for global concurrency limiting
    semaphore = asyncio.Semaphore(200)

    async with aiohttp.ClientSession() as session:

        # ═══════════════════════════════════════════════════════════════════
        # PHASE 1: Generate Responses (Async)
        # ═══════════════════════════════════════════════════════════════════
        console.print(Panel("PHASE 1: Generating Responses (Async)", style="bold blue"))
        
        async def process_question(model, category, question, progress, task_id):
            model_id = model["id"]
            messages = [
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": question},
            ]
            
            async with semaphore:
                cached = response_cache.get(model_id, question)
                if cached:
                    cache_hits[0] += 1
                    progress.advance(task_id)
                    return model_id, {
                        "category": category,
                        "question": question,
                        "response": cached["response"],
                        "time": cached["elapsed"],
                        "usage": cached["usage"],
                    }
                
                response, elapsed, usage = await query_model_async(session, model_id, messages)
                response_cache.set(model_id, question, response, elapsed, usage)
                progress.advance(task_id)
                return model_id, {
                    "category": category,
                    "question": question,
                    "response": response,
                    "time": elapsed,
                    "usage": usage,
                }

        with Progress(
            SpinnerColumn(), BarColumn(), TextColumn("[progress.percentage]{task.percentage:>3.0f}%"), 
            TextColumn("• {task.completed}/{task.total} responses"), TimeRemainingColumn(), console=console
        ) as progress:
            task_id = progress.add_task("[cyan]Generating...", total=total_queries)
            tasks = []
            for model in MODELS:
                for category, question in TEST_QUESTIONS:
                    tasks.append(process_question(model, category, question, progress, task_id))
            
            # Run tasks
            completed_responses = await asyncio.gather(*tasks)
            
            # Aggregate results
            for model_id, data in completed_responses:
                results[model_id]["responses"].append(data)
                results[model_id]["total_time"] += data["time"]
        
        response_cache.save_if_dirty()
        console.print(f"[green]✅ Generated {total_queries} responses ({cache_hits[0]} from cache)[/]\n")
        
        # ═══════════════════════════════════════════════════════════════════
        # PHASE 2: Head-to-Head Battles (Elo System)
        # ═══════════════════════════════════════════════════════════════════
        console.print(Panel("PHASE 2: Head-to-Head Battles (Async Elo)", style="bold blue"))
        
        model_ids = [m["id"] for m in MODELS]
        matchups = list(itertools.combinations(model_ids, 2))
        battle_stats = {mid: {"wins": 0, "losses": 0, "ties": 0} for mid in model_ids}
        categories_to_judge = list(TEST_QUESTIONS_BY_CATEGORY.keys())
        judges = MODELS.copy()
        
        battle_tasks = []
        for model_a, model_b in matchups:
            for category in categories_to_judge:
                possible_judges = [j for j in judges if j["id"] not in (model_a, model_b)]
                for judge in possible_judges:
                    battle_tasks.append({
                        "judge": judge, "model_a": model_a, "model_b": model_b,
                        "model_a_name": next(m["name"] for m in MODELS if m["id"] == model_a),
                        "model_b_name": next(m["name"] for m in MODELS if m["id"] == model_b),
                        "category": category
                    })

        async def run_battle(task_data, progress, task_id):
            judge_id = task_data["judge"]["id"]
            model_a = task_data["model_a"]
            model_b = task_data["model_b"]
            category = task_data["category"]
            
            async with semaphore:
                cached_winner = battle_cache.get(judge_id, model_a, model_b, category)
                if cached_winner:
                    progress.advance(task_id)
                    return model_a, model_b, cached_winner, category, True
                
                # Align questions
                res_a_map = {r["question"]: r["response"] for r in results.get(model_a, {}).get("responses", []) 
                             if r.get("category") == category}
                res_b_map = {r["question"]: r["response"] for r in results.get(model_b, {}).get("responses", []) 
                             if r.get("category") == category}
                common = sorted(list(set(res_a_map.keys()) & set(res_b_map.keys())))[:5]
                
                if not common:
                    progress.advance(task_id)
                    return model_a, model_b, "Tie", category, False

                # Check lengths
                len_a = sum(len(res_a_map[q]) for q in common) / len(common)
                len_b = sum(len(res_b_map[q]) for q in common) / len(common)
                if len_a > 400 and len_b > 400: winner = "Tie"
                elif len_a > 400: winner = "model_b"
                elif len_b > 400: winner = "model_a"
                else: winner = None
                
                if winner:
                    battle_cache.set(judge_id, model_a, model_b, category, winner)
                    progress.advance(task_id)
                    return model_a, model_b, winner, category, False

                # Regular Battle
                battle_text = ""
                for i, q in enumerate(common, 1):
                    battle_text += f"Q{i}: \"{q}\"\nModel A: \"{res_a_map[q]}\"\nModel B: \"{res_b_map[q]}\"\n\n"
                
                messages = [
                    {"role": "system", "content": "You are a strict judge. Choose which model vibes better. Respond JSON."},
                    {"role": "user", "content": JUDGE_BATTLE_PROMPT % battle_text}
                ]
                
                judge_resp, _, _ = await query_model_async(session, judge_id, messages, temperature=0, top_p=1.0)
                
                # Parse
                winner = "Tie"
                try:
                    json_match = re.search(r'\{.*\}', judge_resp, re.DOTALL)
                    if json_match:
                         d = json.loads(json_match.group())
                         w_str = d.get("winner", "Tie").strip().upper()
                         if "A" in w_str and "B" not in w_str: winner = "model_a"
                         elif "B" in w_str and "A" not in w_str: winner = "model_b"
                         elif "TIE" in w_str: winner = "Tie"
                except:
                    pass
                
                battle_cache.set(judge_id, model_a, model_b, category, winner)
                progress.advance(task_id)
                return model_a, model_b, winner, category, False

        with Progress(
            SpinnerColumn(), BarColumn(), TextColumn("[progress.percentage]{task.percentage:>3.0f}%"), 
            TextColumn("• {task.completed}/{task.total} battles"), TimeRemainingColumn(), console=console
        ) as progress:
            task_id = progress.add_task("[cyan]Running battles...", total=len(battle_tasks))
            tasks = [run_battle(t, progress, task_id) for t in battle_tasks]
            
            # Execute battles
            results_battles = await asyncio.gather(*tasks)
            
            for m_a, m_b, winner, _, _ in results_battles:
                if winner == "model_a":
                    battle_stats[m_a]["wins"] += 1
                    battle_stats[m_b]["losses"] += 1
                elif winner == "model_b":
                    battle_stats[m_b]["wins"] += 1
                    battle_stats[m_a]["losses"] += 1
                else:
                    battle_stats[m_a]["ties"] += 1
                    battle_stats[m_b]["ties"] += 1

        battle_cache.save_if_dirty()
        console.print(f"[green]✅ Completed {len(battle_tasks)} battles[/]\n")

    # ═══════════════════════════════════════════════════════════════════
    # PHASE 3: Final Results (Elo Calculation)
    # ═══════════════════════════════════════════════════════════════════
    console.print(Panel("PHASE 3: Final Results (Elo Ratings)", style="bold green"))
    
    # 1. Calculate Global Elo
    initial_elo = 1000
    K = 32
    elo_ratings = {mid: initial_elo for mid in model_ids}
    
    for m_a, m_b, winner, _, _ in results_battles:
        rating_a = elo_ratings[m_a]
        rating_b = elo_ratings[m_b]
        
        expected_a = 1 / (1 + 10 ** ((rating_b - rating_a) / 400))
        expected_b = 1 / (1 + 10 ** ((rating_a - rating_b) / 400))
        
        score_a = 1.0 if winner == "model_a" else 0.5 if winner == "Tie" else 0.0
        score_b = 1.0 - score_a
        
        elo_ratings[m_a] += K * (score_a - expected_a)
        elo_ratings[m_b] += K * (score_b - expected_b)

    # 2. Calculate Per-Category Elo
    category_elos = {cat: {m: 1000.0 for m in model_ids} for cat in categories_to_judge}
    
    for m_a, m_b, winner, cat, _ in results_battles:
        if cat not in category_elos: continue
        
        ratings = category_elos[cat]
        rating_a = ratings[m_a]
        rating_b = ratings[m_b]
        
        expected_a = 1 / (1 + 10 ** ((rating_b - rating_a) / 400))
        expected_b = 1 / (1 + 10 ** ((rating_a - rating_b) / 400))
        
        score_a = 1.0 if winner == "model_a" else 0.5 if winner == "Tie" else 0.0
        score_b = 1.0 - score_a
        
        ratings[m_a] += K * (score_a - expected_a)
        ratings[m_b] += K * (score_b - expected_b)

    # Build Leaderboard
    leaderboard = []
    for model_id, rating in elo_ratings.items():
        model_name = next((m["name"] for m in MODELS if m["id"] == model_id), model_id)
        stats = battle_stats[model_id]
        total_battles = stats["wins"] + stats["losses"] + stats["ties"]
        win_rate = (stats["wins"] / total_battles * 100) if total_battles > 0 else 0
        
        # Get category scores for this model
        cat_scores = {cat: category_elos[cat][model_id] for cat in categories_to_judge}
        
        leaderboard.append({
            "name": model_name,
            "id": model_id,
            "elo": rating,
            "win_rate": win_rate,
            "wins": stats["wins"],
            "losses": stats["losses"],
            "ties": stats["ties"],
            "battles": total_battles,
            "category_elo": cat_scores
        })
    
    leaderboard.sort(key=lambda x: x["elo"], reverse=True)
    
    # Print Main Leaderboard
    lb_table = Table(title="🏆 GLOBAL ELO LEADERBOARD 🏆", box=box.DOUBLE_EDGE, header_style="bold yellow")
    lb_table.add_column("Rank", justify="center", style="bold cyan")
    lb_table.add_column("Model", style="white")
    lb_table.add_column("Global Elo", justify="right", style="bold green")
    lb_table.add_column("Win Rate", justify="right")
    
    for i, item in enumerate(leaderboard, 1):
        medal = "🥇" if i == 1 else "🥈" if i == 2 else "🥉" if i == 3 else f"#{i}"
        lb_table.add_row(
            medal, item["name"], f"{item['elo']:.0f}", f"{item['win_rate']:.1f}%"
        )
    console.print(lb_table)
    console.print()

    # Print Category Matrix
    # We'll split categories if there are too many
    cats = sorted(categories_to_judge)
    chunk_size = 6
    for i in range(0, len(cats), chunk_size):
        chunk = cats[i:i + chunk_size]
        
        cat_matrix = Table(title=f"📊 Category Elo Ratings ({i+1}-{min(i+chunk_size, len(cats))})", box=box.SIMPLE)
        cat_matrix.add_column("Model", style="cyan")
        for cat in chunk:
            cat_matrix.add_column(cat[:10], justify="right") # Truncate header
            
        for item in leaderboard:
            row = [item["name"]]
            for cat in chunk:
                score = item["category_elo"].get(cat, 1000)
                style = "green" if score >= 1100 else "red" if score < 900 else "dim"
                row.append(f"[{style}]{score:.0f}[/]")
            cat_matrix.add_row(*row)
            
        console.print(cat_matrix)
        console.print()
    
    # Performance table
    perf_table = Table(title="📊 Performance Metrics", box=box.ROUNDED)
    perf_table.add_column("Model", style="cyan")
    perf_table.add_column("Avg Time", justify="right")
    perf_table.add_column("Avg Tokens", justify="right")
    perf_table.add_column("Elo", justify="right", style="green")
    
    for model_id, data in results.items():
        if not data["responses"]:
            continue
            
        avg_time = data["total_time"] / len(data["responses"])
        avg_tokens = sum(r.get("usage", {}).get("completion_tokens", 0) for r in data["responses"]) / len(data["responses"])
        elo = elo_ratings.get(model_id, 0)
        
        perf_table.add_row(data['name'], f"{avg_time:.1f}s", f"{avg_tokens:.0f}", f"{elo:.0f}")
    
    console.print(perf_table)
    
    # Save full results
    from datetime import datetime
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    output_path = Path(__file__).parent.parent / ".benchmarks" / f"vibe_benchmark_{timestamp}.json"
    
    full_results = {
        "timestamp": timestamp,
        "responses": results,
        "battle_stats": battle_stats,
        "elo_ratings": elo_ratings,
        "leaderboard": leaderboard,
        "cache_hits": cache_hits[0],
        "total_queries": total_queries,
    }
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(full_results, f, indent=2, ensure_ascii=False)
    
    console.print(f"\n[green]📁 Results saved to: .benchmarks/{output_path.name}[/]")
    
    # Print sample responses for the top model
    if leaderboard:
        # Leaderboard items are dicts now
        top_model_id = leaderboard[0]["id"]
        top_model_name = leaderboard[0]["name"]
        
        console.print()
        console.print(Panel(f"💬 Sample Responses from Winner: {top_model_name}", style="bold magenta"))
        
        for resp in results.get(top_model_id, {}).get("responses", [])[:5]:
            q = resp['question'][:50]
            a = resp['response'][:150].replace('\n', ' ')
            console.print(f"\n  [dim]❓[/] [cyan]\"{q}\"[/]")
            console.print(f"  [green]💬 {a}{'...' if len(resp['response']) > 150 else ''}[/]")
    
    console.print()
    console.print(Panel.fit(
        "[bold magenta]🎀 Benchmark complete! Thank you for using Marin Vibes Benchmark 🎀[/]",
        border_style="magenta",
    ))


def list_categories():
    """Display all available question categories."""
    cat_table = Table(title="📋 Question Categories", box=box.ROUNDED)
    cat_table.add_column("Category", style="cyan")
    cat_table.add_column("Questions", justify="right", style="green")
    
    total = 0
    for cat, questions in TEST_QUESTIONS_BY_CATEGORY.items():
        count = len(questions)
        total += count
        cat_table.add_row(cat, str(count))
    
    cat_table.add_section()
    cat_table.add_row("[bold]TOTAL[/]", f"[bold]{total}[/]")
    
    console.print(cat_table)
    print()


def clear_cache():
    """Clear all benchmark caches."""
    benchmarks_dir = Path(__file__).parent.parent / ".benchmarks"
    
    cleared = []
    for file in benchmarks_dir.glob("*_cache.json"):
        file.unlink()
        cleared.append(file.name)
        
    if cleared:
        print(f"✅ Cleared caches: {', '.join(cleared)}")
    else:
        print("ℹ️  No caches found to clear.")


def main():
    parser = argparse.ArgumentParser(description="Run Marin Vibe Benchmark")
    
    parser.add_argument(
        "--list-categories", "-l",
        action="store_true",
        help="List all available question categories and exit"
    )
    
    parser.add_argument(
        "--clear-cache", "-c",
        action="store_true",
        help="Clear the response cache and exit"
    )
    
    parser.add_argument(
        "--clear-battles", "-cb",
        action="store_true",
        help="Clear only the battle/judge cache (keep responses) and exit"
    )
    
    parser.add_argument(
        "--category", "-cat",
        type=str,
        help="Only run questions from a specific category"
    )
    
    args = parser.parse_args()
    
    if args.list_categories:
        list_categories()
        return
    
    if args.clear_cache:
        clear_cache()
        return

    if args.clear_battles:
        benchmarks_dir = Path(__file__).parent.parent / ".benchmarks"
        for f in benchmarks_dir.glob("*battle_cache.json"):
            f.unlink()
        for f in benchmarks_dir.glob("*judge_cache.json"): # Legacy
            f.unlink()
        print("✅ Battle caches cleared (responses preserved)!")
        return
    
    # Filter categories if specified
    if args.category:
        global TEST_QUESTIONS
        if args.category not in TEST_QUESTIONS_BY_CATEGORY:
            print(f"❌ Unknown category: {args.category}")
            return
        TEST_QUESTIONS = [(args.category, q) for q in TEST_QUESTIONS_BY_CATEGORY[args.category]]

    # Run Async Benchmark
    try:
        if sys.platform == 'win32':
             asyncio.set_event_loop_policy(asyncio.WindowsSelectorEventLoopPolicy())
        asyncio.run(run_benchmark())
    except KeyboardInterrupt:
        console.print("\n[red]⚠️ Benchmark interrupted by user.[/]")


if __name__ == "__main__":
    main()
