package shellkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bosley/beau"
	"github.com/bosley/beau/toolkit"
)

// Platform information
type PlatformInfo struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Shell     string `json:"shell"`
	ShellPath string `json:"shell_path"`
	HomeDir   string `json:"home_dir"`
	TempDir   string `json:"temp_dir"`
	PathSep   string `json:"path_separator"`
	IsWindows bool   `json:"is_windows"`
	IsPosix   bool   `json:"is_posix"`
	ShellType string `json:"shell_type"` // bash, zsh, powershell, cmd
}

// detectPlatform gathers information about the current platform
func detectPlatform() PlatformInfo {
	info := PlatformInfo{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		IsWindows: runtime.GOOS == "windows",
		IsPosix:   runtime.GOOS != "windows",
		PathSep:   string(os.PathSeparator),
	}

	// Get home directory
	info.HomeDir, _ = os.UserHomeDir()
	info.TempDir = os.TempDir()

	// Detect shell
	if info.IsWindows {
		// Check for PowerShell first, then cmd
		if path, err := exec.LookPath("powershell.exe"); err == nil {
			info.Shell = "powershell"
			info.ShellPath = path
			info.ShellType = "powershell"
		} else if path, err := exec.LookPath("cmd.exe"); err == nil {
			info.Shell = "cmd"
			info.ShellPath = path
			info.ShellType = "cmd"
		}
	} else {
		// POSIX systems
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		info.ShellPath = shell
		info.Shell = filepath.Base(shell)

		// Determine shell type
		switch info.Shell {
		case "bash":
			info.ShellType = "bash"
		case "zsh":
			info.ShellType = "zsh"
		case "fish":
			info.ShellType = "fish"
		case "sh":
			info.ShellType = "sh"
		default:
			info.ShellType = "sh" // Default to sh compatibility
		}
	}

	return info
}

// CommandResult represents the result of a command execution
type CommandResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration int64  `json:"duration_ms"`
	Command  string `json:"command"`
}

// GetShellKit returns a toolkit with shell command capabilities
func GetShellKit(logger *slog.Logger, callback toolkit.KitCallback, projectBounds []beau.ProjectBounds) *toolkit.LlmToolKit {
	platformInfo := detectPlatform()

	return toolkit.NewKit("Shell Kit").
		WithTool(getExecuteCommandTool(logger, platformInfo, projectBounds)).
		WithTool(getListProcessesTool(logger, platformInfo)).
		WithTool(getEnvironmentTool(logger, platformInfo)).
		WithTool(getWorkingDirectoryTool(logger, platformInfo)).
		WithTool(getSystemInfoTool(logger, platformInfo)).
		WithTool(getScriptTool(logger, platformInfo, projectBounds)).
		WithCallback(callback)
}

// getExecuteCommandTool creates a tool for executing shell commands
func getExecuteCommandTool(logger *slog.Logger, platform PlatformInfo, projectBounds []beau.ProjectBounds) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "execute_command",
			Description: fmt.Sprintf("Execute a shell command on %s using %s. Commands run with a timeout for safety.", platform.OS, platform.Shell),
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": fmt.Sprintf("The command to execute. Use %s syntax.", platform.ShellType),
					},
					"working_dir": map[string]interface{}{
						"type":        "string",
						"description": "Working directory for the command (optional, must be within project bounds)",
					},
					"timeout_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "Command timeout in seconds. Default: 30, max: 300",
					},
					"env_vars": map[string]interface{}{
						"type":        "object",
						"description": "Additional environment variables as key-value pairs",
					},
				},
				"required": []string{"command"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				Command        string            `json:"command"`
				WorkingDir     string            `json:"working_dir"`
				TimeoutSeconds int               `json:"timeout_seconds"`
				EnvVars        map[string]string `json:"env_vars"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Set defaults
			if args.TimeoutSeconds <= 0 {
				args.TimeoutSeconds = 30
			}
			if args.TimeoutSeconds > 300 {
				args.TimeoutSeconds = 300
			}

			// Validate working directory if specified
			if args.WorkingDir != "" && len(projectBounds) > 0 {
				validPath := false
				for _, pb := range projectBounds {
					if strings.HasPrefix(args.WorkingDir, pb.ABSPath) {
						validPath = true
						break
					}
				}
				if !validPath {
					return nil, fmt.Errorf("working directory must be within project bounds")
				}
			}

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(args.TimeoutSeconds)*time.Second)
			defer cancel()

			// Prepare command based on platform
			var cmd *exec.Cmd
			if platform.IsWindows {
				if platform.ShellType == "powershell" {
					cmd = exec.CommandContext(ctx, platform.ShellPath, "-Command", args.Command)
				} else {
					cmd = exec.CommandContext(ctx, platform.ShellPath, "/C", args.Command)
				}
			} else {
				cmd = exec.CommandContext(ctx, platform.ShellPath, "-c", args.Command)
			}

			// Set working directory
			if args.WorkingDir != "" {
				cmd.Dir = args.WorkingDir
			}

			// Set environment variables
			cmd.Env = os.Environ()
			for k, v := range args.EnvVars {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}

			// Capture output
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			// Execute command
			start := time.Now()
			err := cmd.Run()
			duration := time.Since(start).Milliseconds()

			result := CommandResult{
				Command:  args.Command,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Duration: duration,
			}

			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					result.ExitCode = exitErr.ExitCode()
				} else {
					result.ExitCode = -1
				}
			} else {
				result.ExitCode = 0
			}

			return result, nil
		},
	)
}

// getListProcessesTool creates a tool for listing running processes
func getListProcessesTool(logger *slog.Logger, platform PlatformInfo) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "list_processes",
			Description: "List running processes with their PIDs and names",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filter": map[string]interface{}{
						"type":        "string",
						"description": "Filter processes by name (case-insensitive)",
					},
				},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				Filter string `json:"filter"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			var cmd *exec.Cmd
			if platform.IsWindows {
				// Windows: use tasklist
				cmd = exec.Command("tasklist", "/FO", "CSV")
			} else {
				// POSIX: use ps
				cmd = exec.Command("ps", "aux")
			}

			output, err := cmd.Output()
			if err != nil {
				return nil, fmt.Errorf("failed to list processes: %w", err)
			}

			// Parse and filter output
			outputStr := string(output)
			if args.Filter != "" {
				lines := strings.Split(outputStr, "\n")
				var filtered []string
				filterLower := strings.ToLower(args.Filter)
				for _, line := range lines {
					if strings.Contains(strings.ToLower(line), filterLower) {
						filtered = append(filtered, line)
					}
				}
				outputStr = strings.Join(filtered, "\n")
			}

			return map[string]interface{}{
				"processes": outputStr,
				"platform":  platform.OS,
			}, nil
		},
	)
}

func getEnvironmentTool(logger *slog.Logger, platform PlatformInfo) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "get_environment",
			Description: "Get environment variables, optionally filtered by pattern",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"pattern": map[string]interface{}{
						"type":        "string",
						"description": "Filter environment variables by pattern (e.g., 'PATH', 'HOME')",
					},
					"show_all": map[string]interface{}{
						"type":        "boolean",
						"description": "Show all environment variables. Default: false (shows common ones)",
					},
				},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				Pattern string `json:"pattern"`
				ShowAll bool   `json:"show_all"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			envVars := make(map[string]string)

			// Common environment variables to show by default
			commonVars := []string{
				"PATH", "HOME", "USER", "SHELL", "PWD", "LANG", "TERM",
				"GOPATH", "GOROOT", "JAVA_HOME", "PYTHON_PATH",
				"NODE_PATH", "npm_config_prefix",
			}

			if platform.IsWindows {
				commonVars = append(commonVars, "USERPROFILE", "COMPUTERNAME", "OS", "PROCESSOR_ARCHITECTURE")
			}

			for _, env := range os.Environ() {
				parts := strings.SplitN(env, "=", 2)
				if len(parts) != 2 {
					continue
				}

				key := parts[0]
				value := parts[1]

				// Apply filter if specified
				if args.Pattern != "" {
					if !strings.Contains(strings.ToLower(key), strings.ToLower(args.Pattern)) {
						continue
					}
				} else if !args.ShowAll {
					// If not showing all, only show common vars
					isCommon := false
					for _, common := range commonVars {
						if key == common {
							isCommon = true
							break
						}
					}
					if !isCommon {
						continue
					}
				}

				envVars[key] = value
			}

			return envVars, nil
		},
	)
}

func getWorkingDirectoryTool(logger *slog.Logger, platform PlatformInfo) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "get_working_directory",
			Description: "Get the current working directory and list its contents",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"list_contents": map[string]interface{}{
						"type":        "boolean",
						"description": "List directory contents. Default: true",
					},
				},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				ListContents *bool `json:"list_contents"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			listContents := true
			if args.ListContents != nil {
				listContents = *args.ListContents
			}

			cwd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("failed to get working directory: %w", err)
			}

			result := map[string]interface{}{
				"path": cwd,
			}

			if listContents {
				entries, err := os.ReadDir(cwd)
				if err == nil {
					var files []string
					var dirs []string
					for _, entry := range entries {
						if entry.IsDir() {
							dirs = append(dirs, entry.Name()+"/")
						} else {
							files = append(files, entry.Name())
						}
					}
					result["directories"] = dirs
					result["files"] = files
				}
			}

			return result, nil
		},
	)
}

func getSystemInfoTool(logger *slog.Logger, platform PlatformInfo) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "get_system_info",
			Description: "Get detailed system and platform information",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		func(input []byte) (interface{}, error) {
			// Get additional runtime info
			info := map[string]interface{}{
				"platform":      platform,
				"go_version":    runtime.Version(),
				"num_cpu":       runtime.NumCPU(),
				"num_goroutine": runtime.NumGoroutine(),
			}

			// Add PATH information
			pathSep := ":"
			if platform.IsWindows {
				pathSep = ";"
			}
			paths := strings.Split(os.Getenv("PATH"), pathSep)
			info["path_dirs"] = paths

			return info, nil
		},
	)
}

func getScriptTool(logger *slog.Logger, platform PlatformInfo, projectBounds []beau.ProjectBounds) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "create_script",
			Description: fmt.Sprintf("Create an executable shell script for %s", platform.OS),
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path where to save the script (must be within project bounds)",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": fmt.Sprintf("Script content in %s syntax", platform.ShellType),
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Description comment to add at the top of the script",
					},
				},
				"required": []string{"file_path", "content"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				FilePath    string `json:"file_path"`
				Content     string `json:"content"`
				Description string `json:"description"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Validate path is within bounds
			if len(projectBounds) > 0 {
				validPath := false
				for _, pb := range projectBounds {
					if strings.HasPrefix(args.FilePath, pb.ABSPath) {
						validPath = true
						break
					}
				}
				if !validPath {
					return nil, fmt.Errorf("script path must be within project bounds")
				}
			}

			// Build script content
			var scriptContent strings.Builder

			if platform.IsWindows {
				if platform.ShellType == "powershell" {
					scriptContent.WriteString("# PowerShell Script\n")
					if args.Description != "" {
						scriptContent.WriteString(fmt.Sprintf("# %s\n", args.Description))
					}
					scriptContent.WriteString(fmt.Sprintf("# Generated on %s\n\n", time.Now().Format(time.RFC3339)))
				} else {
					scriptContent.WriteString("@echo off\n")
					if args.Description != "" {
						scriptContent.WriteString(fmt.Sprintf("REM %s\n", args.Description))
					}
					scriptContent.WriteString(fmt.Sprintf("REM Generated on %s\n\n", time.Now().Format(time.RFC3339)))
				}
			} else {
				// POSIX shell
				scriptContent.WriteString(fmt.Sprintf("#!/usr/bin/env %s\n", platform.Shell))
				if args.Description != "" {
					scriptContent.WriteString(fmt.Sprintf("# %s\n", args.Description))
				}
				scriptContent.WriteString(fmt.Sprintf("# Generated on %s\n", time.Now().Format(time.RFC3339)))
				scriptContent.WriteString("# Platform: " + platform.OS + "/" + platform.Arch + "\n\n")
			}

			scriptContent.WriteString(args.Content)

			// Ensure proper file extension
			ext := filepath.Ext(args.FilePath)
			if ext == "" {
				if platform.IsWindows {
					if platform.ShellType == "powershell" {
						args.FilePath += ".ps1"
					} else {
						args.FilePath += ".bat"
					}
				} else {
					args.FilePath += ".sh"
				}
			}

			// Write the script file
			err := os.WriteFile(args.FilePath, []byte(scriptContent.String()), 0755)
			if err != nil {
				return nil, fmt.Errorf("failed to write script: %w", err)
			}

			return map[string]interface{}{
				"path":     args.FilePath,
				"size":     len(scriptContent.String()),
				"platform": platform.OS,
				"shell":    platform.Shell,
				"message":  fmt.Sprintf("Script created successfully at %s", args.FilePath),
			}, nil
		},
	)
}
