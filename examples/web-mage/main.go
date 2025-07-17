package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bosley/beau"
	"github.com/bosley/beau/mage"
)

func main() {
	// Set up logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Get API key from environment
	apiKey := os.Getenv("XAI_API_KEY")
	if apiKey == "" {
		log.Fatal("XAI_API_KEY environment variable is required")
	}

	// Get current working directory for project bounds
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal("Failed to get current directory:", err)
	}

	// Create portal configuration
	portalConfig := mage.PortalConfig{
		Logger:       logger,
		APIKey:       apiKey,
		BaseURL:      beau.DefaultBaseURL_XAI,
		HTTPClient:   &http.Client{},
		RetryConfig:  beau.DefaultRetryConfig(),
		PrimaryModel: beau.DefaultModel_XAI,
		ImageModel:   beau.DefaultModel_XAI,
		MiniModel:    beau.DefaultModel_XAI,
		ProjectBounds: []beau.ProjectBounds{
			{
				Name:        "example",
				Description: "Example project directory",
				ABSPath:     cwd,
			},
		},
		MaxTokens:   8192,
		Temperature: 0.7,
	}

	// Create portal
	portal := mage.NewPortal(portalConfig)

	// Summon the web mage
	webMage, err := portal.Summon(mage.Mage_WB)
	if err != nil {
		log.Fatal("Failed to summon web mage:", err)
	}

	// Add context
	err = webMage.AddToContext("You are helping to capture screenshots of websites for documentation purposes.")
	if err != nil {
		log.Fatal("Failed to add context:", err)
	}

	// Example commands to try
	commands := []string{
		"Navigate to https://insula.dev and take a fullpage screenshot",
		"Capture https://news.ycombinator.com with a viewport screenshot, wait 3 seconds for it to load",
		"Go to https://golang.org and capture the whole page",
	}

	// Execute commands
	for i, cmd := range commands {
		fmt.Printf("\n=== Command %d: %s ===\n", i+1, cmd)

		ctx := context.Background()
		result, err := webMage.Execute(ctx, cmd)
		if err != nil {
			log.Printf("Error executing command: %v\n", err)
			continue
		}

		fmt.Printf("Result: %s\n", result)
	}

	// Show where screenshots were saved
	screenshotDir := filepath.Join(cwd, ".web", "screenshots")
	fmt.Printf("\n=== Screenshots saved to: %s ===\n", screenshotDir)

	// List screenshots
	entries, err := os.ReadDir(screenshotDir)
	if err == nil {
		fmt.Println("\nSaved files:")
		for _, entry := range entries {
			if !entry.IsDir() {
				fmt.Printf("  - %s\n", entry.Name())
			}
		}
	}
}
