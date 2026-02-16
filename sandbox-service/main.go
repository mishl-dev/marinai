package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	workspaceDir   = "/workspace"
	defaultTimeout = 30 * time.Second
	resetInterval  = 10 * time.Minute
)

var (
	isResetting    atomic.Bool
	activeRequests sync.WaitGroup
	lastRequest    atomic.Int64
)

type ExecRequest struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type ExecResponse struct {
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

type WriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func main() {
	os.MkdirAll(workspaceDir, 0755)

	lastRequest.Store(time.Now().Unix())

	go startAutoReset()
	go startIdleChecker()

	http.HandleFunc("/exec", handleExec)
	http.HandleFunc("/read", handleRead)
	http.HandleFunc("/write", handleWrite)
	http.HandleFunc("/list", handleList)
	http.HandleFunc("/health", handleHealth)

	log.Println("Sandbox service starting on :3002")
	log.Fatal(http.ListenAndServe(":3002", nil))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	lastRequest.Store(time.Now().Unix())
	if isResetting.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "resetting"})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleExec(w http.ResponseWriter, r *http.Request) {
	lastRequest.Store(time.Now().Unix())
	if isResetting.Load() {
		http.Error(w, "service resetting", http.StatusServiceUnavailable)
		return
	}

	activeRequests.Add(1)
	defer activeRequests.Done()

	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	timeout := time.Duration(req.Timeout) * time.Second
	if timeout <= 0 || timeout > defaultTimeout {
		timeout = defaultTimeout
	}

	if isDangerous(req.Command) {
		http.Error(w, "command not allowed", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", req.Command)
	cmd.Dir = workspaceDir
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/workspace"}

	output, err := cmd.CombinedOutput()

	resp := ExecResponse{Output: string(output)}
	if ctx.Err() == context.DeadlineExceeded {
		resp.TimedOut = true
		resp.Error = "command timed out"
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			resp.ExitCode = exitErr.ExitCode()
		}
		if resp.Error == "" {
			resp.Error = err.Error()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleRead(w http.ResponseWriter, r *http.Request) {
	lastRequest.Store(time.Now().Unix())
	if isResetting.Load() {
		http.Error(w, "service resetting", http.StatusServiceUnavailable)
		return
	}

	activeRequests.Add(1)
	defer activeRequests.Done()

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(workspaceDir, filepath.Base(path))
	if !isWithinWorkspace(fullPath) {
		http.Error(w, "invalid path", http.StatusForbidden)
		return
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
}

func handleWrite(w http.ResponseWriter, r *http.Request) {
	lastRequest.Store(time.Now().Unix())
	if isResetting.Load() {
		http.Error(w, "service resetting", http.StatusServiceUnavailable)
		return
	}

	activeRequests.Add(1)
	defer activeRequests.Done()

	var req WriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(workspaceDir, filepath.Base(req.Path))
	if !isWithinWorkspace(fullPath) {
		http.Error(w, "invalid path", http.StatusForbidden)
		return
	}

	if len(req.Content) > 1024*1024 {
		http.Error(w, "file too large (max 1MB)", http.StatusBadRequest)
		return
	}

	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "path": fullPath})
}

func handleList(w http.ResponseWriter, r *http.Request) {
	lastRequest.Store(time.Now().Unix())
	if isResetting.Load() {
		http.Error(w, "service resetting", http.StatusServiceUnavailable)
		return
	}

	activeRequests.Add(1)
	defer activeRequests.Done()

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	fullPath := filepath.Join(workspaceDir, path)
	if !isWithinWorkspace(fullPath) {
		http.Error(w, "invalid path", http.StatusForbidden)
		return
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	type FileInfo struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size"`
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, _ := entry.Info()
		files = append(files, FileInfo{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func isWithinWorkspace(path string) bool {
	abs, _ := filepath.Abs(path)
	return filepath.Dir(abs) == workspaceDir || abs == workspaceDir ||
		len(abs) > len(workspaceDir) && abs[:len(workspaceDir)+1] == workspaceDir+"/"
}

func isDangerous(cmd string) bool {
	dangerous := []string{"rm -rf", "dd", "mkfs", "format", "> /dev/", "chmod 777", "curl | sh", "wget | sh"}
	for _, d := range dangerous {
		if contains(cmd, d) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func startAutoReset() {
	ticker := time.NewTicker(resetInterval)
	for range ticker.C {
		log.Println("Starting scheduled reset...")
		reset()
		log.Println("Reset complete")
	}
}

func startIdleChecker() {
	const idleTimeout = 5 * time.Minute
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		lastActive := time.Unix(lastRequest.Load(), 0)
		if time.Since(lastActive) > idleTimeout {
			log.Println("Idle timeout reached, shutting down...")
			os.Exit(0)
		}
	}
}

func reset() {
	isResetting.Store(true)
	activeRequests.Wait()
	os.RemoveAll(workspaceDir)
	os.MkdirAll(workspaceDir, 0755)
	isResetting.Store(false)
}
