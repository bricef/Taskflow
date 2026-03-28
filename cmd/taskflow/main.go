package main

import (
	"fmt"
	"os"

	"github.com/bricef/taskflow/internal/cli"
)

func main() {
	cfg := cli.Config{
		ServerURL: envOr("TASKFLOW_URL", "http://localhost:8374"),
		APIKey:    os.Getenv("TASKFLOW_API_KEY"),
	}

	root := cli.BuildCLI(cfg)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
