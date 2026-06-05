package main

import (
	"bytes"
	"context"
	"encoding/json"
	_ "embed"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

//go:embed index.html
var indexHTML []byte

type RunRequest struct {
	Code string `json:"code"`
}

type RunResponse struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

var port = envInt("PORT", 8080)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/run", handleRun)
	mux.HandleFunc("/health", handleHealth)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	addr := ":" + strconv.Itoa(port)
	log.Printf("Starting Rux Playground backend on %s", addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      withCORS(mux),
		ReadTimeout:  35 * time.Second,
		WriteTimeout: 35 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, RunResponse{
			Success: false,
			Error:   "invalid JSON: " + err.Error(),
		})
		return
	}

	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, RunResponse{
			Success: false,
			Error:   "code field is required",
		})
		return
	}

	resp := runCode(req.Code)
	writeJSON(w, http.StatusOK, resp)
}

func runCode(code string) RunResponse {
	tmpDir, err := os.MkdirTemp("", "rux-play-*")
	if err != nil {
		return RunResponse{Success: false, Error: "failed to create temp dir: " + err.Error()}
	}
	os.Chmod(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "Main.rux")
	if err := os.WriteFile(srcPath, []byte(code), 0644); err != nil {
		return RunResponse{Success: false, Error: "failed to write source: " + err.Error()}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--memory=128m",
		"--cpus=0.5",
		"--security-opt=no-new-privileges",
		"--cap-drop=ALL",
		"--network=none",
		"-v", tmpDir+":/workspace:ro",
		"rux-playground-img",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	resp := RunResponse{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMs: elapsed.Milliseconds(),
	}

	if ctx.Err() == context.DeadlineExceeded {
		resp.Success = false
		resp.Error = "execution timed out (30s limit)"
	} else if runErr != nil {
		resp.Success = false
		resp.Error = runErr.Error()
	} else {
		resp.Success = true
	}

	return resp
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
