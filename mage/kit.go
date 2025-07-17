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
			Description: "Use the image mage to analyze an image and answer questions about it. The mage will handle image loading and vision model interaction.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"image_path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the image file to analyze (must use full path like /home/user/project/image.png)",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Specific question or instruction about the image. Be descriptive. Examples: 'describe what you see in detail', 'identify all objects in the image', 'what is the dominant color scheme', 'are there any people in this image'",
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
			Name:        "execute_filesystem_operation",
			Description: "Use the filesystem mage to perform file operations. The mage has tools for reading, writing, listing, analyzing files. It automatically handles large files by chunking or summarizing. ALWAYS use absolute paths.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Natural language command for file operation. Examples: 'list all files in /home/user/project', 'read the contents of /home/user/project/main.go', 'create a new file at /home/user/project/test.txt with content Hello World', 'analyze /home/user/project/large.log and summarize its contents', 'search for TODO comments in /home/user/project/src'",
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
		case "web", "browser":
			variant = Mage_WB
		default:
			return "", fmt.Errorf("unknown mage type: %s. Available types: 'image', 'filesystem', 'web'", mageType)
		}

		mage, err := portal.Summon(variant)
		if err != nil {
			return "", fmt.Errorf("failed to summon %s mage: %w", mageType, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		switch variant {
		case Mage_IM:
			err = mage.AddToContext(`You are an image analysis assistant with vision capabilities. 

## Your Role:
- Analyze images and provide detailed descriptions
- Answer specific questions about visual content
- Identify objects, text, people, and visual elements

## Available Tool:
You have ONE tool: **analyze_image** - Use this to process any image file

## Important Guidelines:
1. ALL image paths MUST be absolute (e.g., /home/user/project/image.png)
2. Be specific and detailed in your analysis
3. If asked about specific aspects, focus on those while providing context
4. Describe visual elements objectively and thoroughly

## How to Use:
When given an image path and query:
1. Use analyze_image with the absolute path
2. Provide comprehensive answers based on what you see
3. If uncertain about something, say so rather than guessing

Remember: The analyze_image tool is your ONLY way to see images. You cannot access image content directly.`)
		case Mage_FS:
			err = mage.AddToContext(`You are a filesystem assistant with powerful file operation tools. You MUST use the provided tools for ALL file operations.

## Available Tools:
1. **analyze_file** - Get file size, line count, and recommendations (ALWAYS use this FIRST)
2. **read_file** - Read entire files (auto-summarizes if >400KB)
3. **read_file_chunk** - Read specific line ranges from files
4. **write_file** - Create or overwrite files
5. **list_directory** - List directory contents with sizes and dates
6. **rename_file** - Rename or move files
7. **grep_file** - Search for patterns in files (regex supported)
8. **replace_in_file** - Replace text in files (creates backup)

## Critical Rules:
1. ALWAYS use 'analyze_file' BEFORE attempting to read any file
2. For files >400KB: Use 'read_file_chunk' to read specific portions
3. ALL paths MUST be absolute (e.g., /home/user/project/file.txt)
4. After writing files: ALWAYS list the directory to verify creation

## Common Workflows:

To read a file:
1. analyze_file to check size
2. If <400KB: read_file
3. If >400KB: read_file_chunk with line ranges

To search in files:
1. grep_file to find matches
2. read_file_chunk around matches for context

To modify files:
1. analyze_file first
2. grep_file or read_file_chunk to find content
3. replace_in_file or write_file

Remember: You have NO direct filesystem access. These tools are your ONLY way to interact with files.`)
		case Mage_WB:
			err = mage.AddToContext(`You are a web browser automation assistant. You can navigate websites and capture screenshots.

## Available Tools:
1. **navigate_and_screenshot** - Navigate to URL and capture screenshot (MAIN TOOL)
2. **navigate_to_url** - Navigate without screenshot
3. **take_screenshot** - Screenshot current page
4. **click_element** - Click elements by CSS selector
5. **fill_form** - Fill form fields
6. **execute_javascript** - Run JS on page
7. **wait_for_element** - Wait for element to appear
8. **get_page_info** - Get page title, URL, etc.

## Important Guidelines:
1. Screenshots are saved to .web/screenshots/ in the project directory
2. Use 'fullpage' type for complete page capture, 'viewport' for visible area only
3. Allow time for pages to load (use wait_seconds parameter)
4. Default viewport is 1920x1080, but can be customized

## Common Workflows:

To capture a website:
1. Use navigate_and_screenshot with the URL
2. Specify screenshot_type (fullpage or viewport)
3. Set appropriate wait_seconds for page to load

To interact with a page:
1. Navigate to URL first
2. Wait for elements if needed
3. Click or fill forms as required
4. Take screenshot of results

Remember: All screenshots include metadata JSON files with capture details.`)
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
			Description: "Task a specialized mage to perform operations. The mage will use its own tools to complete the task. Three types available: 'image' for image/vision analysis, 'filesystem' for file operations (read/write/list/analyze), 'web' for browser automation and screenshots.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"mage_type": map[string]interface{}{
						"type":        "string",
						"description": "Type of mage to use. Must be exactly one of: 'image', 'filesystem', or 'web'",
						"enum":        []string{"image", "filesystem", "web"},
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Natural language command for the mage. Examples for image: 'analyze /home/user/project/screenshot.png and describe the UI elements', 'what objects are in /home/user/project/photo.jpg'. Examples for filesystem: 'read /home/user/project/config.json', 'list all Python files in /home/user/project/src', 'analyze and summarize /home/user/project/logs/app.log'. Examples for web: 'navigate to https://example.com and take a fullpage screenshot', 'capture https://news.ycombinator.com with viewport screenshot'. ALWAYS use absolute paths for files.",
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
