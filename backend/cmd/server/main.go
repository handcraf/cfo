package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cfo/backend/internal/api"
	"github.com/cfo/backend/internal/config"
	"github.com/cfo/backend/internal/service"
	"github.com/cfo/backend/internal/storage"
	"github.com/cfo/backend/internal/storage/sqlstore"
)

func main() {
	cfg := config.Load()

	if err := storage.InitDirectories(cfg.DataDir); err != nil {
		log.Fatalf("Failed to initialize storage directories: %v", err)
	}

	// Stage 3: open SQLite source-of-truth. Non-fatal on failure so the
	// legacy JSON-only path still works in demos.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var sqlStore *sqlstore.Store
	if cfg.SQLiteEnabled {
		if err := os.MkdirAll(filepath.Dir(cfg.SQLitePath), 0o755); err != nil {
			log.Printf("[main] sqlite dir mkdir: %v (continuing without SQL)", err)
		} else {
			s, err := sqlstore.Open(ctx, cfg.SQLitePath)
			if err != nil {
				log.Printf("[main] sqlite open failed: %v (continuing without SQL)", err)
			} else {
				sqlStore = s
				defer sqlStore.Close()
				if res, err := sqlStore.MigrateFromJSON(ctx, cfg.DataDir); err != nil {
					log.Printf("[main] sqlite migrate: %v", err)
				} else {
					log.Printf("[main] sqlite migrate result: %+v", res)
				}
			}
		}
	}

	// Build vector backend. In-memory is the default; Qdrant is opt-in
	// via VECTOR_BACKEND=qdrant.
	vs, vsCloser := buildVectorStore(ctx, cfg)
	if vsCloser != nil {
		defer vsCloser()
	}

	// -----------------------------------------------------------------
	// License + auth gate.
	//
	// Per spec: "License validation must happen in backend startup
	// before AI services become usable." We do not refuse to bind the
	// port — instead, we bind and let the gate middleware return 503
	// on every business route. That way the frontend can still load
	// /license/status and render the LicenseError page.
	// -----------------------------------------------------------------
	verifyRes, verifier, authSvc, licState := licenseStartup(cfg.DataDir)
	gate := api.NewLicenseAuthHandler(verifier, authSvc, verifyRes, licState)

	mux := api.SetupRoutesWithDeps(cfg, api.Deps{
		SQL:         sqlStore,
		VectorStore: vs,
		Gate:        gate,
	})

	addr := ":" + cfg.Port
	srv := &http.Server{Addr: addr, Handler: mux}

	log.Printf("Starting AI CFO backend on %s", addr)
	log.Printf("Data directory: %s", cfg.DataDir)
	log.Printf("LLM runtime: llama.cpp binary=%s model=%s (ctx=%d, temp=%.2f, top_p=%.2f)",
		cfg.LlamaCppBinary, cfg.ModelPath, cfg.LLMContextSize, cfg.LLMTemperature, cfg.LLMTopP)
	log.Printf("SQLite: enabled=%v path=%s", cfg.SQLiteEnabled, cfg.SQLitePath)
	log.Printf("Vector backend: %s", cfg.VectorBackend)
	if cfg.OllamaHost != "" {
		log.Printf("[main] legacy Ollama embedding host set: %s (deprecated)", cfg.OllamaHost)
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM so WAL files land cleanly.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	log.Printf("[main] shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[main] shutdown: %v", err)
	}
}

// buildVectorStore constructs the vector backend selected by cfg. Returns
// (store, closer); closer may be nil if the store has nothing to close.
//
// Failures are logged and the store is nil, which the api layer treats as
// "keyword-only RAG" — the system stays usable.
func buildVectorStore(ctx context.Context, cfg *config.Config) (service.Store, func()) {
	switch cfg.VectorBackend {
	case "qdrant":
		embedder := service.NewEmbeddingService(service.EmbeddingConfig{
			OllamaHost: cfg.OllamaHost,
			ModelName:  "nomic-embed-text",
		})
		q := service.NewQdrantStore(service.QdrantConfig{
			BaseURL:    cfg.QdrantURL,
			Collection: cfg.QdrantCollection,
			APIKey:     cfg.QdrantAPIKey,
			Embedder:   embedder,
			VectorDim:  cfg.EmbeddingDim,
		})
		if err := q.EnsureCollection(ctx); err != nil {
			log.Printf("[main] qdrant ensure collection: %v (disabling vector store)", err)
			return nil, nil
		}
		log.Printf("[main] vector store: qdrant at %s (coll=%s)", cfg.QdrantURL, cfg.QdrantCollection)
		return q, func() { _ = q.Close() }
	default:
		// In-memory JSON-backed store. No embedder wired here: the ask
		// handler falls back to keyword RAG when embeddings are missing.
		// TODO: wire EmbeddingService to in-memory store when ready to
		// enable semantic search in the default deployment.
		log.Printf("[main] vector store: in-memory")
		return nil, nil
	}
}
