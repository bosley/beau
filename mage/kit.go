package mage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/bosley/beau"
	"github.com/bosley/beau/toolkit"
)

/*
Builtds and returns the operational tool kit that can
be handed used to get definitions that will be handed directly
to the LLM (GetTools())
*/

type MageKitConfig struct {
	Logger    *slog.Logger
	Callback  toolkit.KitCallback
	ImageMage Mage
	FSMage    Mage
}

// GetMageKit builds and returns a toolkit that allows LLMs to task the mages
func GetMageKit(config MageKitConfig) *toolkit.LlmToolKit {
	return toolkit.NewKit("Mage Kit").
		WithTool(getImageAnalysisTool(config)).
		WithTool(getFilesystemTool(config)).
		WithCallback(config.Callback)
}

func getImageAnalysisTool(config MageKitConfig) toolkit.LlmTool {
	analyzeWithMage := func(imagePath string, query string) (string, error) {
		if config.ImageMage == nil {
			return "", fmt.Errorf("image mage not available")
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Format the command for the image mage
		command := fmt.Sprintf("Please analyze the image at path '%s' and %s", imagePath, query)

		// Execute via the image mage
		result, err := config.ImageMage.Execute(ctx, command)
		if err != nil {
			return "", fmt.Errorf("image mage execution failed: %w", err)
		}

		return result, nil
	}

	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "analyze_image_with_mage",
			Description: "Use the image mage to analyze an image and answer questions about it",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"image_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the image file to analyze",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Specific question or instruction about the image (e.g., 'describe what you see', 'identify the objects', 'what is the dominant color')",
					},
				},
				"required": []string{"image_path", "query"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				ImagePath string `json:"image_path"`
				Query     string `json:"query"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("failed to parse arguments: %w", err)
			}

			if args.ImagePath == "" {
				return nil, fmt.Errorf("image_path is required")
			}
			if args.Query == "" {
				args.Query = "describe what you see in detail"
			}

			return analyzeWithMage(args.ImagePath, args.Query)
		},
	)
}

func getFilesystemTool(config MageKitConfig) toolkit.LlmTool {
	executeFileOperation := func(command string) (string, error) {
		if config.FSMage == nil {
			return "", fmt.Errorf("filesystem mage not available")
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Execute via the filesystem mage
		result, err := config.FSMage.Execute(ctx, command)
		if err != nil {
			return "", fmt.Errorf("filesystem mage execution failed: %w", err)
		}

		return result, nil
	}

	return toolkit.NewTool(
		beau.ToolSchema{
			Name: "execute_filesystem_operation",
			Description: `
			Use the filesystem mage to perform file operations like reading, writing, listing directories, or analyzing files.
			The mage will handle large files appropriately by chunking or summarizing.`,
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The filesystem operation command to execute (e.g., 'list files in the current directory', 'analyze file.txt then read it appropriately', 'create a new file called test.txt with content Hello World', 'summarize the large.log file')",
					},
				},
				"required": []string{"command"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("failed to parse arguments: %w", err)
			}

			if args.Command == "" {
				return nil, fmt.Errorf("command is required")
			}

			return executeFileOperation(args.Command)
		},
	)
}

// GetUnifiedMageKit creates a toolkit that provides a single interface to task any mage
func GetUnifiedMageKit(portal *Portal, logger *slog.Logger, callback toolkit.KitCallback) *toolkit.LlmToolKit {
	return toolkit.NewKit("Unified Mage Kit").
		WithTool(getUnifiedMageTool(portal, logger)).
		WithCallback(callback)
}

func getUnifiedMageTool(portal *Portal, logger *slog.Logger) toolkit.LlmTool {
	executeMageTask := func(mageType string, command string) (string, error) {
		var variant MageVariant
		switch mageType {
		case "image", "vision":
			variant = Mage_IM
		case "filesystem", "fs", "file":
			variant = Mage_FS
		default:
			return "", fmt.Errorf("unknown mage type: %s. Available types: 'image', 'filesystem'", mageType)
		}

		mage, err := portal.Summon(variant)
		if err != nil {
			return "", fmt.Errorf("failed to summon %s mage: %w", mageType, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		switch variant {
		case Mage_IM:
			err = mage.AddToContext("You are a helpful assistant that analyzes images and provides detailed descriptions.")
		case Mage_FS:
			err = mage.AddToContext(`You are a helpful assistant with file system access capabilities. Always use the provided functions for file operations.

IMPORTANT: When working with files:
1. ALWAYS use 'analyze_file' first to check file size before attempting to read
2. For files larger than 400KB, use 'read_file_chunk' to read specific portions instead of 'read_file'
3. When summarizing large files, read them in chunks using 'read_file_chunk' with appropriate line ranges
4. The 'read_file' tool will automatically provide a summary for files over 400KB, but it's better to use 'read_file_chunk' for controlled reading`)
		}

		if err != nil {
			return "", fmt.Errorf("failed to add context: %w", err)
		}

		// Execute the command
		if logger != nil {
			logger.Info("Executing mage task", "type", mageType, "command", command)
		}

		result, err := mage.Execute(ctx, command)
		if err != nil {
			// Check if context was cancelled
			if ctx.Err() != nil {
				return "", fmt.Errorf("mage execution cancelled: %w", ctx.Err())
			}
			return "", fmt.Errorf("mage execution failed: %w", err)
		}

		return result, nil
	}

	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "task_mage",
			Description: "Task a specialized mage to perform operations. Available mages: 'image' for image analysis, 'filesystem' for file operations.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"mage_type": map[string]interface{}{
						"type":        "string",
						"description": "The type of mage to use: 'image', 'filesystem'",
						"enum":        []string{"image", "filesystem"},
					},
					"command": map[string]interface{}{
						"type": "string",
						"description": `The command or task for the mage to execute. Examples: Image: 'analyze image.png', 
						Filesystem: 'analyze and summarize large.log'. For filesystem operations with large files, 
						the mage will automatically handle them appropriately.`,
					},
				},
				"required": []string{"mage_type", "command"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				MageType string `json:"mage_type"`
				Command  string `json:"command"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("failed to parse arguments: %w", err)
			}

			return executeMageTask(args.MageType, args.Command)
		},
	)
}
