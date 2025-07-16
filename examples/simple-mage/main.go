package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bosley/beau"
	"github.com/bosley/beau/mage"
	"github.com/fatih/color"
)

func main() {

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	baseUrl := beau.DefaultBaseURL_XAI
	defaultModel := "grok-3" // THIS IS CORRECT it JUST released
	apiKeyVar := "XAI_API_KEY"
	maxTokens := 8000
	maxTemperature := 0.7
	maxRetries := 5
	directory := "."
	httpTimeout := 5 * time.Minute
	skipTLSVerification := false

	flag.StringVar(&baseUrl, "base-url", baseUrl, "The base URL for the Beau API")
	flag.StringVar(&apiKeyVar, "api-key-var", apiKeyVar, "The environment variable for the Beau API key")
	flag.IntVar(&maxTokens, "max-tokens", maxTokens, "The maximum number of tokens to generate")
	flag.Float64Var(&maxTemperature, "max-temperature", maxTemperature, "The maximum temperature for the Beau API")
	flag.IntVar(&maxRetries, "max-retries", maxRetries, "The maximum number of retries for the Beau API")
	flag.StringVar(&defaultModel, "model", defaultModel, "The default model to use")
	flag.StringVar(&directory, "dir", directory, "The directory to use")
	flag.BoolVar(&skipTLSVerification, "skip-tls-verification", skipTLSVerification, "Skip TLS verification")
	flag.DurationVar(&httpTimeout, "http-timeout", httpTimeout, "The timeout for HTTP requests")
	flag.Parse()

	retryConfig := beau.RetryConfig{
		MaxRetries:    maxRetries,
		InitialDelay:  time.Second * 2,
		MaxDelay:      time.Second * 60,
		BackoffFactor: 2.0,
		Enabled:       true,
	}

	apiKey := os.Getenv(apiKeyVar)
	if apiKey == "" {
		color.HiRed("API key not found in environment variable %s", apiKeyVar)
		os.Exit(1)
	}

	httpClient := &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: skipTLSVerification,
			},
		},
	}

	dirAsABS, err := filepath.Abs(directory)
	if err != nil {
		color.HiRed("Failed to get absolute path for directory %s: %v", directory, err)
		os.Exit(1)
	}

	portal := mage.NewPortal(mage.PortalConfig{
		Logger:       logger,
		APIKey:       apiKey,
		BaseURL:      baseUrl,
		MaxTokens:    maxTokens,
		Temperature:  maxTemperature,
		HTTPClient:   httpClient,
		RetryConfig:  retryConfig,
		PrimaryModel: defaultModel,
		ImageModel:   defaultModel,
		ProjectBounds: []beau.ProjectBounds{
			{
				Name:        "project",
				Description: "The project directory",
				ABSPath:     dirAsABS,
			},
		},
	})

	tulpa, err := portal.Summon(mage.Mage_FS)
	if err != nil {
		color.HiRed("Failed to summon mage: %v", err)
		os.Exit(1)
	}

	tulpa.AddToContext("You are a helpful assistant that can help with tasks related to the project directory.")

	result, err := tulpa.Execute(context.Background(), "What files do you see. DOnt read them. Just list them.")
	if err != nil {
		color.HiRed("Failed to execute mage: %v", err)
		os.Exit(1)
	}

	color.HiGreen("Result: %s", result)

}
