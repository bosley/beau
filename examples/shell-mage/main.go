package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"runtime"

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

	// Summon the shell mage
	shellMage, err := portal.Summon(mage.Mage_SH)
	if err != nil {
		log.Fatal("Failed to summon shell mage:", err)
	}

	// Add context
	err = shellMage.AddToContext("You are helping to demonstrate shell command capabilities.")
	if err != nil {
		log.Fatal("Failed to add context:", err)
	}

	fmt.Printf("=== Shell Mage Example ===\n")
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Working Directory: %s\n\n", cwd)

	// Example commands - platform aware
	var commands []string

	if runtime.GOOS == "windows" {
		commands = []string{
			"Show me the system information",
			"List all files in the current directory",
			"Show me the environment variables related to PATH",
			"Create a batch script that says hello and shows the date",
		}
	} else {
		commands = []string{
			"Show me the system information",
			"Execute 'ls -la' to see all files with details",
			"Show me the PATH environment variable",
			"Create a shell script that prints a greeting and the current date",
			"List processes that contain 'go' in their name",
		}
	}

	// Execute commands
	ctx := context.Background()
	for i, cmd := range commands {
		fmt.Printf("\n=== Command %d: %s ===\n", i+1, cmd)

		result, err := shellMage.Execute(ctx, cmd)
		if err != nil {
			log.Printf("Error executing command: %v\n", err)
			continue
		}

		fmt.Printf("Result:\n%s\n", result)
	}

	fmt.Println("\n=== Shell Mage Example Complete ===")
}
