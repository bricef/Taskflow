package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/service"
	"github.com/bricef/taskflow/internal/sqlite"
	"github.com/bricef/taskflow/internal/taskflow"
	"github.com/bricef/taskflow/internal/webhook"

	taskflowhttp "github.com/bricef/taskflow/internal/http"
)

func main() {
	dbPath := envOr("TASKFLOW_DB_PATH", "./taskflow.db")
	listenAddr := envOr("TASKFLOW_LISTEN_ADDR", ":8374")

	if os.Getenv("TASKFLOW_DEV_MODE") == "true" {
		model.AllowPrivateWebhookURLs = true
		log.Println("Development mode enabled: private webhook URLs allowed")
	}

	store, err := sqlite.New(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer store.Close()

	bus := eventbus.New()
	svc := service.New(store, service.WithEventBus(bus))

	if err := seedAdmin(svc); err != nil {
		log.Fatalf("failed to seed admin: %v", err)
	}

	// Start webhook dispatcher.
	whDispatcher := webhook.NewDispatcher(bus, svc, store)
	defer whDispatcher.Stop()

	srv := taskflowhttp.NewServer(svc, taskflowhttp.ServerConfig{
		EventBus:           bus,
		MaxRequestBodyBytes: envInt64("TASKFLOW_MAX_BODY_BYTES", 0),
		ReadTimeout:         envDuration("TASKFLOW_READ_TIMEOUT", 0),
		WriteTimeout:        envDuration("TASKFLOW_WRITE_TIMEOUT", 0),
		IdleTimeout:         envDuration("TASKFLOW_IDLE_TIMEOUT", 0),
		RateLimitPerSecond:  envInt("TASKFLOW_RATE_LIMIT", 0),
	})

	log.Printf("TaskFlow server listening on %s", listenAddr)
	if err := srv.ListenAndServe(listenAddr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// seedAdmin creates the initial admin actor if TASKFLOW_SEED_ADMIN_NAME is set
// and no actors exist yet.
func seedAdmin(svc taskflow.TaskFlow) error {
	name := os.Getenv("TASKFLOW_SEED_ADMIN_NAME")
	if name == "" {
		return nil
	}

	actors, err := svc.ListActors(context.Background())
	if err != nil {
		return err
	}
	if len(actors) > 0 {
		return nil // actors already exist, skip seeding
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		return fmt.Errorf("generating API key: %w", err)
	}

	displayName := envOr("TASKFLOW_SEED_ADMIN_DISPLAY_NAME", name)

	_, err = svc.CreateActor(context.Background(), model.CreateActorParams{
		Name:        name,
		DisplayName: displayName,
		Type:        model.ActorTypeHuman,
		Role:        model.RoleAdmin,
		APIKeyHash:  taskflowhttp.HashAPIKey(apiKey),
	})
	if err != nil {
		return fmt.Errorf("creating seed admin: %w", err)
	}

	keyFile := envOr("TASKFLOW_SEED_KEY_FILE", "./seed-admin-key.txt")
	if err := os.WriteFile(keyFile, []byte(apiKey+"\n"), 0600); err != nil {
		return fmt.Errorf("writing seed admin key file: %w", err)
	}

	log.Printf("Seed admin %q created. API key written to %s", name, keyFile)
	return nil
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
