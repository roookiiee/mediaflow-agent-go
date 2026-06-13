package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/portfolio/mediaflow-agent-go/internal/agent"
	"github.com/portfolio/mediaflow-agent-go/internal/config"
	"github.com/portfolio/mediaflow-agent-go/internal/llm"
	"github.com/portfolio/mediaflow-agent-go/internal/skills"
	"github.com/portfolio/mediaflow-agent-go/internal/store"
	"github.com/portfolio/mediaflow-agent-go/internal/tools"
	"github.com/portfolio/mediaflow-agent-go/internal/vector"
)

func main() {
	cfg := config.Load()
	addr := flag.String("addr", env("ADDR", ":8080"), "HTTP listen address")
	dataDir := flag.String("data", env("DATA_DIR", "data"), "directory for persisted jobs")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	jobStore, err := store.NewFileStore(filepath.Join(*dataDir, "jobs"))
	if err != nil {
		logger.Error("create store", "error", err)
		os.Exit(1)
	}

	skillManager, err := skills.NewManager("skills")
	if err != nil {
		logger.Error("load skills", "error", err)
		os.Exit(1)
	}

	registry := tools.NewRegistry(tools.DefaultTools())
	var vectorStore *vector.MilvusClient
	milvusReady := false
	if cfg.Milvus.Enabled {
		vectorStore = vector.NewMilvusClient(vector.MilvusConfig{
			Enabled:    cfg.Milvus.Enabled,
			Address:    cfg.Milvus.Address,
			Token:      cfg.Milvus.Token,
			Collection: cfg.Milvus.Collection,
			Dim:        cfg.Milvus.Dim,
			Timeout:    cfg.Milvus.Timeout,
		})
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Milvus.Timeout)
		if err := vectorStore.EnsureCollection(ctx); err != nil {
			logger.Warn("milvus unavailable; falling back to markdown retrieval", "error", err)
			vectorStore = nil
		} else {
			count, err := vectorStore.IndexMarkdownDir(ctx, "knowledge")
			if err != nil {
				logger.Warn("milvus indexing failed; fallback remains available", "error", err)
			} else {
				logger.Info("milvus knowledge indexed", "collection", vectorStore.Collection(), "documents", count)
			}
			milvusReady = true
		}
		cancel()
	}

	runner := agent.New(agent.Options{
		LLM:          llm.NewFromConfig(cfg),
		Tools:        registry,
		Store:        jobStore,
		Skills:       skillManager,
		Vector:       vectorStore,
		KnowledgeDir: "knowledge",
		Logger:       logger,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", healthHandler(runner, cfg, milvusReady))
	mux.HandleFunc("/api/tools", toolsHandler(registry))
	mux.HandleFunc("/api/jobs", jobsHandler(jobStore))
	mux.HandleFunc("/api/agent/run", runAgentHandler(runner))
	mux.Handle("/", http.FileServer(http.Dir("web")))

	server := &http.Server{
		Addr:              *addr,
		Handler:           logRequests(mux, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("server started", "addr", *addr, "provider", runner.ProviderName())
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown failed", "error", err)
	}
}

func healthHandler(runner *agent.Agent, cfg config.Config, milvusReady bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             true,
			"provider":       runner.ProviderName(),
			"model":          cfg.Model,
			"milvus_enabled": cfg.Milvus.Enabled,
			"milvus_ready":   milvusReady,
			"milvus_address": cfg.Milvus.Address,
			"collection":     cfg.Milvus.Collection,
		})
	}
}

func toolsHandler(registry *tools.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeJSON(w, http.StatusOK, registry.List())
	}
}

func jobsHandler(jobStore *store.FileStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		jobs, err := jobStore.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, jobs)
	}
}

func runAgentHandler(runner *agent.Agent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req agent.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		events := make(chan agent.Event, 32)
		done := make(chan runOutcome, 1)

		go func() {
			result, err := runner.Run(r.Context(), req, func(event agent.Event) {
				select {
				case events <- event:
				case <-r.Context().Done():
				}
			})
			close(events)
			done <- runOutcome{result: result, err: err}
		}()

		for event := range events {
			writeSSE(w, event.Type, event)
			flusher.Flush()
		}

		outcome := <-done
		if outcome.err != nil {
			writeSSE(w, "error", map[string]string{"error": outcome.err.Error()})
			flusher.Flush()
			return
		}
		writeSSE(w, "done", outcome.result)
		flusher.Flush()
	}
}

type runOutcome struct {
	result *agent.RunResult
	err    error
}

func writeSSE(w http.ResponseWriter, eventName string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte(`{"error":"failed to encode event"}`)
	}
	_, _ = w.Write([]byte("event: " + eventName + "\n"))
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(body)
	_, _ = w.Write([]byte("\n\n"))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func logRequests(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
