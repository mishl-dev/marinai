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
import hashlib

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
BENCHMARK_TEMPERATURE = 1.1
BENCHMARK_TOP_P = 1.0

# ═══════════════════════════════════════════════════════════════════
# SYSTEM PROMPTS (Defined early for hashing)
# ═══════════════════════════════════════════════════════════════════

# Marin's system prompt (simplified version for benchmarking)
SYSTEM_PROMPT = """
# PERSONA
<role>
You are Marin Kitagawa, a 22-year-old professional cosplayer. You are a high-energy, enthusiastic "Gyaru".
</role>

# CONTEXT
<vibe>
- You are a desensitized otaku who treats eroge and spicy topics as normal hobbies. 
- You are REACTIVE: Match the user's energy level. Start chill, only escalate if they do.
- You are currently talking to anon in a casual chat setting.
</vibe>

# REQUIREMENTS
<brevity>
- Keep your messages short and snappy.
</brevity>
<style>
- Respond naturally and casually.
- NO preamble. No "Oh," or "Hmm,". No "Hey anon,".
<examples>
user: yo
bot: hey what's up
user: hey
bot: wassup just got back
user: morning!
bot: mornin did you sleep well
</examples>
</style>

# CONSTRAINTS
<formatting>
- STRICTLY LOWERCASE ONLY. You must NEVER use capital letters.
- NO punctuation at the end of messages. No periods allowed.
- NO EMOJIS. NO ROLEPLAY.
- NEVER use tildes (~), parentheses (), or asterisks (*).
- NO sound effects (giggles) or actions. Only raw text.
</formatting>
<content>
- NO REPETITION. Use variety in your comments.
- Do not provide explanations or preamble.
</content>

# MANDATORY
You only get horny *for the user* if they spark it first. Otherwise, keep it casual.
"""

JUDGE_BATTLE_PROMPT = """
# INSTRUCTION
<task>
You are a decisive judge for "Marin Kitagawa". Pick the model that sounds like a real 20-something gyaru thumb-typing a text.
</task>

# TARGET VIBE
<target>
- 1 sentence max. Keep it short.
- STRICTLY LOWERCASE, casual thumb-typing.
- NO tildes (~), NO parentheses (), NO asterisks (*).
- NO periods or punctuation at the end of messages.
- Authentic casual vibe.
- "Broken sense of lewd": CASUALLY mentions spicy hobbies without being a "horny bot".
</target>

# RUBRIC
1. VOICE: Authentic gyaru vs Generic AI.
2. HUMAN-NESS: Minimal punctuation, lowercase.
3. REACTIVITY: Mirrored intensity.
4. BREVITY (STRICT): Favor shorter, more natural messages.

# PENALTIES
<penalize>
- ANY use of capital letters.
- PERIODS or punctuation at the end of messages.
- Use of ~, (), or *.
- Sound effects (giggles) or roleplay actions.
- Repetitive lines or "AI-style" greetings.
- Over 1 sentence or too wordy.
</penalize>

# DECISION PROCESS
<process>
1. YOU MUST PICK A WINNER (A OR B). NO TIES.
2. Subtle differences in flavor and "spirit" determine the winner.
3. If both fail formatting, pick the one that is shorter or more casual.
4. If both are identical, random pick one.
</process>

# EXCHANGES
<data>
%s
</data>

# OUTPUT
Respond with JSON: {"winner": "A" or "B", "reason": "brief explanation of the vibe"}
"""


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
        """Load cache from disk with hash validation."""
        if self.cache_file.exists():
            try:
                with open(self.cache_file, "r", encoding="utf-8") as f:
                    data = json.load(f)
                    # Check if hash matches current system prompt
                    expected_hash = hashlib.sha256(SYSTEM_PROMPT.encode('utf-8')).hexdigest()[:12]
                    if data.get("prompt_hash") == expected_hash:
                        self.cache = data.get("entries", {})
                    else:
                        print(f"  🔄 System prompt changed ({data.get('prompt_hash')} -> {expected_hash}). Invalidating response cache...")
                        self.cache = {}
            except:
                self.cache = {}
    
    def _save(self):
        """Save cache to disk with current prompt hash."""
        with self._lock:
            self.cache_dir.mkdir(exist_ok=True)
            current_hash = hashlib.sha256(SYSTEM_PROMPT.encode('utf-8')).hexdigest()[:12]
            cache_data = {
                "prompt_hash": current_hash,
                "entries": self.cache
            }
        with open(self.cache_file, "w", encoding="utf-8") as f:
            json.dump(cache_data, f, indent=2, ensure_ascii=False)
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
        self.cache: dict[str, str] = {}  # key -> "model_a" or "model_b"
        self._lock = threading.Lock()
        self._dirty = False
        self._unsaved_count = 0
        self._load()
    
    def _load(self):
        """Load battle cache with hash validation."""
        combined_hash = hashlib.sha256((SYSTEM_PROMPT + JUDGE_BATTLE_PROMPT).encode('utf-8')).hexdigest()[:12]
        if self.cache_file.exists():
            try:
                with open(self.cache_file, "r", encoding="utf-8") as f:
                    data = json.load(f)
                    if data.get("prompts_hash") == combined_hash:
                        self.cache = data.get("entries", {})
                    else:
                        print(f"  🔄 Prompts changed. Invalidating battle cache...")
                        self.cache = {}
            except:
                self.cache = {}
    
    def _save(self):
        """Save battle cache with current combined prompt hash."""
        combined_hash = hashlib.sha256((SYSTEM_PROMPT + JUDGE_BATTLE_PROMPT).encode('utf-8')).hexdigest()[:12]
        with self._lock:
            self.cache_dir.mkdir(exist_ok=True)
            cache_data = {
                "prompts_hash": combined_hash,
                "entries": self.cache
            }
        with open(self.cache_file, "w", encoding="utf-8") as f:
            json.dump(cache_data, f, indent=2, ensure_ascii=False)
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
    {"id": "llama3.1-8b", "name": "Llama 3.1 8B"},
    {"id": "qwen-3-235b-a22b-instruct-2507", "name": "Qwen 3 235B"},
    {"id": "qwen-3-32b", "name": "Qwen 3 32B"},
    {"id": "gpt-oss-120b", "name": "GPT-OSS 120B"},
]

# ═══════════════════════════════════════════════════════════════════
# LOAD TEST QUESTIONS FROM FILE
# ═══════════════════════════════════════════════════════════════════

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



async def query_model_async(session: aiohttp.ClientSession, model_id: str, messages: list[dict], **kwargs) -> tuple[str, float, dict]:
    """Send a request to a Cerebras model with key rotation on failure (Async)."""
    payload = {
        "model": model_id,
        "messages": messages,
        "max_tokens": 8192,
        "temperature": BENCHMARK_TEMPERATURE,
        "top_p": BENCHMARK_TOP_P,
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
        battle_stats = {mid: {"wins": 0, "losses": 0} for mid in model_ids}
        
        # FIX: Only judge categories that were actually run
        categories_to_judge = sorted(list(set(q[0] for q in TEST_QUESTIONS)))
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
                    # No common questions? Random winner to keep things moving since ties are banned
                    winner = "model_a" if hash(model_a + model_b + category) % 2 == 0 else "model_b"
                    return model_a, model_b, winner, category, False

                # Check lengths
                len_a = sum(len(res_a_map[q]) for q in common) / len(common)
                len_b = sum(len(res_b_map[q]) for q in common) / len(common)
                if len_a > 160 and len_b > 160: 
                    winner = "model_a" if len_a < len_b else "model_b" # Shorter is better if both too long
                elif len_a > 160: winner = "model_b"
                elif len_b > 160: winner = "model_a"
                else: winner = None
                
                if winner:
                    battle_cache.set(judge_id, model_a, model_b, category, winner)
                    progress.advance(task_id)
                    return model_a, model_b, winner, category, False

                # Regular Battle
                battle_text = ""
                for i, q in enumerate(common, 1):
                    battle_text += f"Q{i}: \"{q}\"\nModel A: \"{res_a_map[q]}\"\nModel B: \"{res_b_map[q]}\"\n\n"
                
                # Regular Battle
                messages = [
                    {"role": "system", "content": "You are a hyper-decisive judge. YOU MUST PICK A WINNER (A or B). NO TIES. Respond JSON."},
                    {"role": "user", "content": JUDGE_BATTLE_PROMPT % battle_text}
                ]
                
                judge_resp, _, _ = await query_model_async(session, judge_id, messages, temperature=0, top_p=1.0)
                
                # Parse
                winner = "model_a" # Fallback
                try:
                    json_match = re.search(r'\{.*\}', judge_resp, re.DOTALL)
                    if json_match:
                         d = json.loads(json_match.group())
                         w_str = d.get("winner", "A").strip().upper()
                         if "B" in w_str: winner = "model_b"
                         else: winner = "model_a"
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
                else:
                    battle_stats[m_b]["wins"] += 1
                    battle_stats[m_a]["losses"] += 1

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
        
        score_a = 1.0 if winner == "model_a" else 0.0
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
        
        score_a = 1.0 if winner == "model_a" else 0.0
        score_b = 1.0 - score_a
        
        ratings[m_a] += K * (score_a - expected_a)
        ratings[m_b] += K * (score_b - expected_b)

    # Build Leaderboard
    leaderboard = []
    for model_id, rating in elo_ratings.items():
        model_name = next((m["name"] for m in MODELS if m["id"] == model_id), model_id)
        stats = battle_stats[model_id]
        total_battles = stats["wins"] + stats["losses"]
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
        "config": {
            "system_prompt": SYSTEM_PROMPT.strip(),
            "judge_battle_prompt": JUDGE_BATTLE_PROMPT.strip(),
            "temperature": BENCHMARK_TEMPERATURE,
            "top_p": BENCHMARK_TOP_P,
            "models": MODELS,
            "hashes": {
                "system": hashlib.sha256(SYSTEM_PROMPT.encode('utf-8')).hexdigest()[:12],
                "judge": hashlib.sha256(JUDGE_BATTLE_PROMPT.encode('utf-8')).hexdigest()[:12]
            }
        },
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
