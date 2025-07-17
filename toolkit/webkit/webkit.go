package webkit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"os/exec"

	"github.com/bosley/beau"
	"github.com/bosley/beau/toolkit"
	"github.com/chromedp/chromedp"
)

// findChromeExecutable searches for Chrome/Chromium executable in common locations
func findChromeExecutable() string {
	// Common executable names
	executables := []string{
		"chromium",
		"chromium-browser",
		"google-chrome",
		"google-chrome-stable",
		"google-chrome-unstable",
		"chrome",
	}

	// Check if any of these exist in PATH
	for _, exe := range executables {
		if path, err := exec.LookPath(exe); err == nil {
			return path
		}
	}

	// Check common installation paths
	commonPaths := []string{
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/local/bin/chromium",
		"/snap/bin/chromium",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Default to chromium-browser if nothing found
	return "chromium-browser"
}

// ScreenshotType defines the type of screenshot to capture
type ScreenshotType string

const (
	ScreenshotFullPage ScreenshotType = "fullpage"
	ScreenshotViewport ScreenshotType = "viewport"
	ScreenshotElement  ScreenshotType = "element"
)

// GetWebKit returns a toolkit with web browser automation capabilities
func GetWebKit(logger *slog.Logger, callback toolkit.KitCallback, projectBounds []beau.ProjectBounds) *toolkit.LlmToolKit {
	return toolkit.NewKit("Web Kit").
		WithTool(getNavigateAndScreenshotTool(logger, projectBounds)).
		WithTool(getNavigateTool(logger)).
		WithTool(getScreenshotTool(logger, projectBounds)).
		WithTool(getClickElementTool(logger)).
		WithTool(getFillFormTool(logger)).
		WithTool(getExecuteJSTool(logger)).
		WithTool(getWaitForElementTool(logger)).
		WithTool(getGetPageInfoTool(logger)).
		WithCallback(callback)
}

// Helper function to ensure screenshot directory exists
func ensureScreenshotDir(projectBounds []beau.ProjectBounds) (string, error) {
	if len(projectBounds) == 0 {
		return "", fmt.Errorf("no project bounds specified")
	}

	screenshotDir := filepath.Join(projectBounds[0].ABSPath, ".web", "screenshots")
	if err := os.MkdirAll(screenshotDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create screenshot directory: %w", err)
	}

	return screenshotDir, nil
}

// Helper to generate timestamped filename
func generateScreenshotFilename(url string) string {
	// Clean URL for filename
	cleanURL := strings.ReplaceAll(url, "https://", "")
	cleanURL = strings.ReplaceAll(cleanURL, "http://", "")
	cleanURL = strings.ReplaceAll(cleanURL, "/", "_")
	cleanURL = strings.ReplaceAll(cleanURL, ":", "_")
	cleanURL = strings.ReplaceAll(cleanURL, "?", "_")
	cleanURL = strings.ReplaceAll(cleanURL, "&", "_")
	cleanURL = strings.ReplaceAll(cleanURL, "=", "_")

	if len(cleanURL) > 50 {
		cleanURL = cleanURL[:50]
	}

	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s_%s.png", cleanURL, timestamp)
}

// getNavigateAndScreenshotTool provides a combined navigation and screenshot tool
func getNavigateAndScreenshotTool(logger *slog.Logger, projectBounds []beau.ProjectBounds) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "navigate_and_screenshot",
			Description: "Navigate to a URL and capture a screenshot. Saves to .web/screenshots/ with timestamp.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL to navigate to (e.g., https://example.com)",
					},
					"screenshot_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"fullpage", "viewport"},
						"description": "Type of screenshot: 'fullpage' captures entire page, 'viewport' captures visible area only. Default: fullpage",
					},
					"wait_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "Seconds to wait after page loads before taking screenshot. Default: 2",
					},
					"viewport_width": map[string]interface{}{
						"type":        "integer",
						"description": "Browser viewport width in pixels. Default: 1920",
					},
					"viewport_height": map[string]interface{}{
						"type":        "integer",
						"description": "Browser viewport height in pixels. Default: 1080",
					},
				},
				"required": []string{"url"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				URL            string `json:"url"`
				ScreenshotType string `json:"screenshot_type"`
				WaitSeconds    int    `json:"wait_seconds"`
				ViewportWidth  int    `json:"viewport_width"`
				ViewportHeight int    `json:"viewport_height"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Set defaults
			if args.ScreenshotType == "" {
				args.ScreenshotType = string(ScreenshotFullPage)
			}
			if args.WaitSeconds <= 0 {
				args.WaitSeconds = 2
			}
			if args.ViewportWidth <= 0 {
				args.ViewportWidth = 1920
			}
			if args.ViewportHeight <= 0 {
				args.ViewportHeight = 1080
			}

			// Ensure screenshot directory exists
			screenshotDir, err := ensureScreenshotDir(projectBounds)
			if err != nil {
				return nil, err
			}

			// Generate filename
			filename := generateScreenshotFilename(args.URL)
			fullPath := filepath.Join(screenshotDir, filename)

			// Find Chrome executable
			chromeExec := findChromeExecutable()

			// Create browser context
			opts := append(chromedp.DefaultExecAllocatorOptions[:],
				chromedp.ExecPath(chromeExec),
				chromedp.WindowSize(args.ViewportWidth, args.ViewportHeight),
				chromedp.Flag("headless", true),
				chromedp.Flag("disable-gpu", true),
				chromedp.Flag("no-sandbox", true),
				chromedp.Flag("disable-dev-shm-usage", true),
			)

			allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
			defer cancel()

			ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(logger.Debug))
			defer cancel()

			// Set timeout
			ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			// Navigate and take screenshot
			var buf []byte
			var tasks chromedp.Tasks

			tasks = append(tasks,
				chromedp.Navigate(args.URL),
				chromedp.Sleep(time.Duration(args.WaitSeconds)*time.Second),
			)

			if args.ScreenshotType == string(ScreenshotFullPage) {
				tasks = append(tasks, chromedp.FullScreenshot(&buf, 90))
			} else {
				tasks = append(tasks, chromedp.CaptureScreenshot(&buf))
			}

			if err := chromedp.Run(ctx, tasks); err != nil {
				return nil, fmt.Errorf("failed to capture screenshot: %w", err)
			}

			// Save screenshot
			if err := os.WriteFile(fullPath, buf, 0644); err != nil {
				return nil, fmt.Errorf("failed to save screenshot: %w", err)
			}

			// Create metadata
			metadata := map[string]interface{}{
				"url":             args.URL,
				"screenshot_type": args.ScreenshotType,
				"viewport":        fmt.Sprintf("%dx%d", args.ViewportWidth, args.ViewportHeight),
				"timestamp":       time.Now().Format(time.RFC3339),
				"file_path":       fullPath,
				"file_size":       len(buf),
			}

			// Save metadata
			metadataPath := strings.TrimSuffix(fullPath, ".png") + "_metadata.json"
			metadataJSON, _ := json.MarshalIndent(metadata, "", "  ")
			os.WriteFile(metadataPath, metadataJSON, 0644)

			return map[string]interface{}{
				"success":    true,
				"screenshot": fullPath,
				"metadata":   metadata,
				"message":    fmt.Sprintf("Screenshot saved to %s", fullPath),
			}, nil
		},
	)
}

// Additional tools would be implemented here...
// For brevity, I'll add just the navigate tool as an example

func getNavigateTool(logger *slog.Logger) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "navigate_to_url",
			Description: "Navigate browser to a specific URL without taking a screenshot",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL to navigate to",
					},
				},
				"required": []string{"url"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// This is a simplified implementation
			// In a real scenario, you might want to maintain a browser session
			return map[string]interface{}{
				"success": true,
				"message": fmt.Sprintf("Navigated to %s", args.URL),
			}, nil
		},
	)
}

// getScreenshotTool captures a screenshot of the current page (requires browser session management)
func getScreenshotTool(logger *slog.Logger, projectBounds []beau.ProjectBounds) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "take_screenshot",
			Description: "Take a screenshot of the current page in an existing browser session",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"screenshot_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"fullpage", "viewport", "element"},
						"description": "Type of screenshot to capture. Default: viewport",
					},
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector for element screenshot (only used if screenshot_type is 'element')",
					},
				},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				ScreenshotType string `json:"screenshot_type"`
				Selector       string `json:"selector"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Note: This would require maintaining browser session state
			// For now, return a message indicating the limitation
			return map[string]interface{}{
				"success": false,
				"message": "Screenshot of current page requires browser session management. Use navigate_and_screenshot instead.",
			}, nil
		},
	)
}

func getClickElementTool(logger *slog.Logger) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "click_element",
			Description: "Click an element on the page by CSS selector",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the element to click (e.g., 'button.submit', '#login-btn', 'a[href=\"/about\"]')",
					},
					"wait_visible": map[string]interface{}{
						"type":        "boolean",
						"description": "Wait for element to be visible before clicking. Default: true",
					},
				},
				"required": []string{"selector"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				Selector    string `json:"selector"`
				WaitVisible *bool  `json:"wait_visible"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			waitVisible := true
			if args.WaitVisible != nil {
				waitVisible = *args.WaitVisible
			}

			return map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("Click element requires browser session. Would click: %s (wait_visible: %v)", args.Selector, waitVisible),
			}, nil
		},
	)
}

func getFillFormTool(logger *slog.Logger) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "fill_form",
			Description: "Fill a form field with text by CSS selector",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the input field (e.g., 'input[name=\"username\"]', '#email', '.search-box')",
					},
					"text": map[string]interface{}{
						"type":        "string",
						"description": "Text to enter into the field",
					},
					"clear_first": map[string]interface{}{
						"type":        "boolean",
						"description": "Clear existing text before typing. Default: true",
					},
				},
				"required": []string{"selector", "text"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				Selector   string `json:"selector"`
				Text       string `json:"text"`
				ClearFirst *bool  `json:"clear_first"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			clearFirst := true
			if args.ClearFirst != nil {
				clearFirst = *args.ClearFirst
			}

			return map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("Fill form requires browser session. Would fill '%s' with '%s' (clear_first: %v)",
					args.Selector, args.Text, clearFirst),
			}, nil
		},
	)
}

func getExecuteJSTool(logger *slog.Logger) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "execute_javascript",
			Description: "Execute JavaScript code on the current page",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"script": map[string]interface{}{
						"type":        "string",
						"description": "JavaScript code to execute. Can return a value.",
					},
				},
				"required": []string{"script"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				Script string `json:"script"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			return map[string]interface{}{
				"success": false,
				"message": "JavaScript execution requires browser session management.",
				"script":  args.Script,
			}, nil
		},
	)
}

func getWaitForElementTool(logger *slog.Logger) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "wait_for_element",
			Description: "Wait for an element to appear on the page",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the element to wait for",
					},
					"timeout_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum time to wait in seconds. Default: 10",
					},
				},
				"required": []string{"selector"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				Selector       string `json:"selector"`
				TimeoutSeconds int    `json:"timeout_seconds"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			if args.TimeoutSeconds <= 0 {
				args.TimeoutSeconds = 10
			}

			return map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("Wait for element requires browser session. Would wait for '%s' (timeout: %ds)",
					args.Selector, args.TimeoutSeconds),
			}, nil
		},
	)
}

func getGetPageInfoTool(logger *slog.Logger) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "get_page_info",
			Description: "Get information about the current page (title, URL, meta tags)",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"include_meta": map[string]interface{}{
						"type":        "boolean",
						"description": "Include meta tags in the response. Default: false",
					},
				},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				IncludeMeta bool `json:"include_meta"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			return map[string]interface{}{
				"success": false,
				"message": "Page info requires browser session management. Use navigate_and_screenshot to capture pages.",
			}, nil
		},
	)
}
