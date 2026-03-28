package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/service"
	"github.com/bricef/taskflow/internal/sqlite"
	"github.com/bricef/taskflow/internal/taskflow"

	taskflowhttp "github.com/bricef/taskflow/internal/http"
)

func main() {
	dbPath := envOr("TASKFLOW_DB_PATH", "./taskflow.db")
	listenAddr := envOr("TASKFLOW_LISTEN_ADDR", ":8374")

	store, err := sqlite.New(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer store.Close()

	svc := service.New(store)

	if err := seedAdmin(svc); err != nil {
		log.Fatalf("failed to seed admin: %v", err)
	}

	srv := taskflowhttp.NewServer(svc)

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
