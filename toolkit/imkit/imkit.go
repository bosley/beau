package imkit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/bosley/beau"
	"github.com/bosley/beau/toolkit"
	"github.com/bosley/beau/toolkit/pathutil"

	"github.com/fatih/color"
)

type TargetVariant string

const (
	Raw  TargetVariant = "raw"  // raws will be a b64 encoded string of the image data
	File TargetVariant = "file" // files will be a path to the image file
)

// GetImageKit returns a toolkit with image analysis capabilities
func GetImageKit(apiKey string, baseURL string, logger *slog.Logger, callback toolkit.KitCallback, model string, projectBounds []beau.ProjectBounds) *toolkit.LlmToolKit {
	return toolkit.NewKit("Image Kit").
		WithTool(getImageAnalysisTool(apiKey, baseURL, logger, model, projectBounds)).
		WithCallback(callback)
}

// getImageAnalysisTool creates a tool for analyzing images with vision models
func getImageAnalysisTool(apiKey string, baseURL string, logger *slog.Logger, model string, projectBounds []beau.ProjectBounds) toolkit.LlmTool {
	analyzeImage := func(variant TargetVariant, target, query string, temperature float64, maxTokens int) (string, error) {
		// Create context for the vision API request
		ctx := context.Background()

		// Configure XAI client with the provided API key
		client, err := beau.NewClient(apiKey, baseURL, nil, logger, beau.RetryConfig{
			MaxRetries:    5,
			InitialDelay:  2 * time.Second,
			MaxDelay:      60 * time.Second,
			BackoffFactor: 2.0,
			Enabled:       true,
		})

		if err != nil {
			return "", fmt.Errorf("failed to create client: %w", err)
		}

		if baseURL != "" {
			client.BaseURL = baseURL
		}
		if logger != nil {
			client = client.WithLogger(logger.WithGroup("image_analysis"))
		}

		// Use default values if not provided
		if temperature <= 0 {
			temperature = beau.DefaultTemperature
		}
		if maxTokens <= 0 {
			maxTokens = beau.DefaultMaxTokens
		}

		// Create a new conversation with the vision model
		visionConversation := client.NewConversation(model,
			beau.WithTemperature(temperature),
			beau.WithMaxTokens(maxTokens),
		)

		// Set system message for the vision model
		visionConversation.AddSystemMessage(`
		You are a helpful assistant that analyzes images.
		Your only task is to analyze the image and return an in-depth description of the image.
		The purpose of your existence is to "understand" the image and relay as much information as possible
		to a higher-order large language model, so make a description of the image that is detailed
		yet concise.
	`)

		var imgBase64, imgType string
		var err2 error

		// Handle different target variants
		switch variant {
		case File:
			// Validate path if project bounds are specified
			if len(projectBounds) > 0 {
				validPath, err := pathutil.ValidatePath(projectBounds, target)
				if err != nil {
					return "", fmt.Errorf("path validation failed: %w", err)
				}
				target = validPath
			}

			imgBase64, imgType, err2 = beau.ReadImageFile(target)
			if err2 != nil {
				return "", fmt.Errorf("failed to read image file: %w", err2)
			}
		case Raw:
			imgBase64 = target
			// For raw data, we assume it's already base64 encoded and try to detect type
			imgType = "image/jpeg" // Default to JPEG if can't determine
			if len(target) > 11 {
				switch target[0:11] {
				case "data:image/p":
					imgType = "image/png"
				case "data:image/j":
					imgType = "image/jpeg"
				}
			}
		default:
			return "", fmt.Errorf("unsupported target variant: %s", variant)
		}

		// Create complex message with both the query and image
		complexMessage := []beau.ContentItem{
			beau.CreateTextItem(query),
			beau.CreateImageBase64Item(imgBase64, imgType, "high"),
		}
		visionConversation.AddComplexUserMessage(complexMessage)

		// Send the request to the vision model
		response, err := visionConversation.Send(ctx, temperature, maxTokens)
		if err != nil {
			return "", fmt.Errorf("vision model error: %w", err)
		}

		color.HiGreen("Response: %s", response.Content)

		// Return the response from the vision model
		return response.Content.(string), nil
	}

	// Create and return the LLM tool definition
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "analyze_image",
			Description: "Analyze an image using a vision model and get a detailed description",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"variant": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"file", "raw"},
						"description": "The type of image input - 'file' for file path or 'raw' for base64 encoded image data",
					},
					"target": map[string]interface{}{
						"type":        "string",
						"description": "Either a file path (when variant='file') or base64 encoded image data (when variant='raw')",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Specific question or instruction about the image (e.g., 'What's in this image?', 'Describe the objects in detail')",
					},
					"temperature": map[string]interface{}{
						"type":        "number",
						"description": "Temperature for the vision model (between 0.0 and 1.0). Higher values make output more random, lower values more deterministic. Default is 0.7 if not specified.",
					},
					"max_tokens": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of tokens to generate. Default is 2000 if not specified.",
					},
				},
				"required": []string{"variant", "target", "query"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				Variant     string  `json:"variant"`
				Target      string  `json:"target"`
				Query       string  `json:"query"`
				Temperature float64 `json:"temperature,omitempty"`
				MaxTokens   int     `json:"max_tokens,omitempty"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("failed to parse arguments: %w", err)
			}

			// Convert variant string to TargetVariant type
			variant := TargetVariant(args.Variant)
			if variant != File && variant != Raw {
				return nil, fmt.Errorf("invalid variant: %s", args.Variant)
			}

			return analyzeImage(variant, args.Target, args.Query, args.Temperature, args.MaxTokens)
		},
	)
}
