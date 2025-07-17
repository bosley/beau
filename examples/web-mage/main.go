package main

import (
	"context"
	"flag"
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
	// Parse command-line flags
	var targetDir string
	flag.StringVar(&targetDir, "target", "", "Target directory where .web/screenshots will be created (required)")
	flag.Parse()

	// Validate required flag
	if targetDir == "" {
		fmt.Fprintf(os.Stderr, "Error: --target flag is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Resolve target directory to absolute path
	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		log.Fatal("Failed to resolve target directory:", err)
	}

	// Check if target directory exists
	if info, err := os.Stat(absTargetDir); err != nil {
		log.Fatal("Target directory does not exist:", err)
	} else if !info.IsDir() {
		log.Fatal("Target path is not a directory:", absTargetDir)
	}

	// Set up logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Get API key from environment
	apiKey := os.Getenv("XAI_API_KEY")
	if apiKey == "" {
		log.Fatal("XAI_API_KEY environment variable is required")
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
				Name:        "target",
				Description: "Target directory for screenshots",
				ABSPath:     absTargetDir,
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

	// Get URLs from remaining arguments or use defaults
	var urls []string
	if len(flag.Args()) > 0 {
		// Use URLs from command line
		for _, arg := range flag.Args() {
			urls = append(urls, fmt.Sprintf("Navigate to %s and take a fullpage screenshot", arg))
		}
	} else {
		// Default examples
		urls = []string{
			"Navigate to https://example.com and take a fullpage screenshot",
			"Capture https://news.ycombinator.com with a viewport screenshot, wait 3 seconds for it to load",
			"Go to https://golang.org and capture the whole page",
		}
	}

	fmt.Printf("Target directory: %s\n", absTargetDir)
	fmt.Printf("Screenshots will be saved to: %s\n", filepath.Join(absTargetDir, ".web", "screenshots"))

	// Execute commands
	for i, cmd := range urls {
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
	screenshotDir := filepath.Join(absTargetDir, ".web", "screenshots")
	fmt.Printf("\n=== Screenshots saved to: %s ===\n", screenshotDir)

	// List screenshots
	entries, err := os.ReadDir(screenshotDir)
	if err == nil {
		fmt.Println("\nSaved files:")
		for _, entry := range entries {
			if !entry.IsDir() {
				info, _ := entry.Info()
				size := ""
				if info != nil {
					size = fmt.Sprintf(" (%d KB)", info.Size()/1024)
				}
				fmt.Printf("  - %s%s\n", entry.Name(), size)
			}
		}
	} else {
		fmt.Printf("Could not list files: %v\n", err)
	}
}
