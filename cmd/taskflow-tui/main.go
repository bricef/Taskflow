package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"

	"github.com/bricef/taskflow/internal/tui"
)

func main() {
	viper.SetEnvPrefix("TASKFLOW")
	viper.AutomaticEnv()
	viper.SetDefault("url", "http://localhost:8374")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.config/taskflow")
	viper.AddConfigPath(".")
	viper.ReadInConfig()

	serverURL := strings.TrimSpace(viper.GetString("url"))
	apiKey := strings.TrimSpace(viper.GetString("api_key"))
	boardSlug := viper.GetString("board")

	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "API key required. Set TASKFLOW_API_KEY or add api_key to ~/.config/taskflow/config.yaml")
		os.Exit(1)
	}

	// If board is passed as positional arg, use it.
	if boardSlug == "" && len(os.Args) > 1 {
		boardSlug = os.Args[1]
	}

	// If a board slug is specified, do a preflight check.
	if boardSlug != "" {
		if err := preflight(serverURL, apiKey, boardSlug); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		// At least check the server is reachable.
		if err := preflightServer(serverURL, apiKey); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	var p *tea.Program
	cfg := tui.Config{
		ServerURL: serverURL,
		APIKey:    apiKey,
		BoardSlug: boardSlug,
		Program:   &p,
	}

	model := tui.New(cfg)
	p = tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func preflight(serverURL, apiKey, boardSlug string) error {
	req, _ := http.NewRequest("GET", serverURL+"/boards/"+boardSlug, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not connect to TaskFlow server at %s\n\nIs the server running?", serverURL)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		return nil
	case 401:
		return fmt.Errorf("authentication failed — check your API key")
	case 404:
		return fmt.Errorf("board %q not found\n\nCreate it first:\n  taskflow board create --slug %s --name \"...\"", boardSlug, boardSlug)
	default:
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		msg := fmt.Sprintf("server returned status %d", resp.StatusCode)
		if m, ok := errResp["message"].(string); ok {
			msg = m
		}
		return fmt.Errorf("%s", msg)
	}
}

func preflightServer(serverURL, apiKey string) error {
	req, _ := http.NewRequest("GET", serverURL+"/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not connect to TaskFlow server at %s\n\nIs the server running?", serverURL)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("server health check failed: status %d", resp.StatusCode)
	}
	return nil
}
