package main

import (
	"bytes"
	"context"
	"encoding/json"
	_ "embed"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
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

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/run", handleRun)
	mux.HandleFunc("/ws", handleWS)
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
		Addr:        addr,
		Handler:     withCORS(mux),
		IdleTimeout: 60 * time.Second,
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

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade: %v", err)
		return
	}
	defer conn.Close()

	_, code, err := conn.ReadMessage()
	if err != nil {
		return
	}

	tmpDir, err := os.MkdirTemp("", "rux-play-*")
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
		return
	}
	defer os.RemoveAll(tmpDir)
	os.Chmod(tmpDir, 0755)

	os.WriteFile(filepath.Join(tmpDir, "Main.rux"), code, 0644)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-i", "--tty",
		"--memory=128m",
		"--cpus=0.5",
		"--security-opt=no-new-privileges",
		"--cap-drop=ALL",
		"--network=none",
		"-v", tmpDir+":/workspace:ro",
		"rux-playground-img",
	)

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 120})
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
		return
	}
	defer f.Close()

	done := make(chan struct{})

	go func() {
		io.Copy(wsWriter{conn}, f)
		close(done)
	}()

	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				f.Close()
				return
			}
			f.Write(msg)
		}
	}()

	<-done
	cmd.Wait()
}

type wsWriter struct{ conn *websocket.Conn }

func (w wsWriter) Write(p []byte) (int, error) {
	err := w.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
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
