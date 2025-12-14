## 2025-12-14 - HTTP Client Timeouts
**Vulnerability:** Use of default `&http.Client{}` for external API calls (Cerebras, Gemini, Embedding Service). The default Go HTTP client has no timeout, meaning a hanging server could cause the bot to hang indefinitely (DoS / Resource Exhaustion).
**Learning:** External services are unreliable. Even "fast" APIs can hang. Go's default `http.Client` is dangerous for production use without explicit timeouts.
**Prevention:** Always set `Timeout` when creating `http.Client`. Use context timeouts for finer control if needed, but a global client timeout is a good baseline defense.
