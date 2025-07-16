package fskit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bosley/beau"
	"github.com/bosley/beau/toolkit"
)

// GetValidatedFsKit returns a filesystem toolkit with path validation
func GetValidatedFsKit(projects []beau.ProjectBounds, callback toolkit.KitCallback) *toolkit.LlmToolKit {
	return toolkit.NewKit("Validated Filesystem Kit").
		WithTool(validatedReadFileTool(projects)).
		WithTool(validatedReadFileChunkTool(projects)).
		WithTool(validatedWriteFileTool(projects)).
		WithTool(validatedListDirectoryTool(projects)).
		WithTool(validatedAnalyzeFileTool(projects)).
		WithTool(validatedRenameFileTool(projects)).
		WithTool(validatedGrepFileTool(projects)).
		WithTool(validatedReplaceInFileTool(projects)).
		WithCallback(callback)
}

// validatePath checks if a path is within allowed project directories
func validatePath(projects []beau.ProjectBounds, path string) (string, error) {
	// Handle empty path as current directory
	if path == "" {
		path = "."
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("could not get absolute path for '%s': %w", path, err)
	}

	// Check each allowed project
	for _, p := range projects {
		projAbsPath, err := filepath.Abs(p.ABSPath)
		if err != nil {
			continue
		}

		// Resolve symlinks to prevent escapes
		resolvedAbsPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			// File doesn't exist yet, check parent
			parentDir := filepath.Dir(absPath)
			resolvedParent, parentErr := filepath.EvalSymlinks(parentDir)
			if parentErr != nil {
				// Walk up to find existing parent
				existingParent := parentDir
				for existingParent != "/" && existingParent != "." {
					if _, statErr := os.Stat(existingParent); statErr == nil {
						if resolved, evalErr := filepath.EvalSymlinks(existingParent); evalErr == nil {
							relPath, _ := filepath.Rel(existingParent, absPath)
							resolvedAbsPath = filepath.Join(resolved, relPath)
							break
						}
					}
					existingParent = filepath.Dir(existingParent)
				}
				if resolvedAbsPath == "" {
					resolvedAbsPath = absPath
				}
			} else {
				resolvedAbsPath = filepath.Join(resolvedParent, filepath.Base(absPath))
			}
		}

		resolvedProjPath, err := filepath.EvalSymlinks(projAbsPath)
		if err != nil {
			return "", fmt.Errorf("could not resolve project path '%s': %w", p.ABSPath, err)
		}

		// Ensure paths end with separator for prefix check
		if !strings.HasSuffix(resolvedProjPath, string(filepath.Separator)) {
			resolvedProjPath += string(filepath.Separator)
		}

		// Check if path is within this project
		if strings.HasPrefix(resolvedAbsPath, resolvedProjPath) ||
			resolvedAbsPath == strings.TrimSuffix(resolvedProjPath, string(filepath.Separator)) {
			return absPath, nil
		}
	}

	// Build error message with allowed paths
	var allowedPaths []string
	for _, p := range projects {
		allowedPaths = append(allowedPaths, fmt.Sprintf("%s (%s)", p.ABSPath, p.Name))
	}

	// Create a more helpful error message
	helpfulPath := ""
	if len(projects) > 0 {
		// Suggest using the first project path with the filename
		helpfulPath = fmt.Sprintf("\nDid you mean: %s/%s", projects[0].ABSPath, filepath.Base(path))
	}

	return "", fmt.Errorf("path '%s' is not within allowed directories: %s%s\nAlways use full absolute paths when working with files",
		path, strings.Join(allowedPaths, ", "), helpfulPath)
}

func validatedReadFileTool(projects []beau.ProjectBounds) toolkit.LlmTool {
	// Define token limits - rough estimate: 1 token ≈ 4 characters
	const (
		maxFileSize  = 400 * 1024 // 400KB - conservative limit for ~100K tokens
		chunkSize    = 100 * 1024 // 100KB chunks
		contextLines = 50         // Lines of context to show for large files
	)

	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "read_file",
			Description: "Read the entire content of a file within the project directories. For large files, provides a summary with beginning and end content.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file to read (e.g., /home/user/project/file.txt)",
					},
				},
				"required": []string{"file_path"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				FilePath string `json:"file_path"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Validate path
			validPath, err := validatePath(projects, args.FilePath)
			if err != nil {
				return nil, err
			}

			// Check file size first
			fileInfo, err := os.Stat(validPath)
			if err != nil {
				return nil, fmt.Errorf("failed to stat file '%s': %w", validPath, err)
			}

			if fileInfo.IsDir() {
				return nil, fmt.Errorf("path '%s' is a directory, not a file", validPath)
			}

			// If file is small enough, read it entirely
			if fileInfo.Size() <= maxFileSize {
				content, err := os.ReadFile(validPath)
				if err != nil {
					return nil, fmt.Errorf("failed to read file '%s': %w", validPath, err)
				}
				return string(content), nil
			}

			// For large files, provide a summary with beginning and end
			file, err := os.Open(validPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open file '%s': %w", validPath, err)
			}
			defer file.Close()

			// Read beginning of file
			beginBuffer := make([]byte, chunkSize)
			n, err := file.Read(beginBuffer)
			if err != nil && err != io.EOF {
				return nil, fmt.Errorf("failed to read beginning of file: %w", err)
			}
			beginContent := string(beginBuffer[:n])

			// Read end of file
			endBuffer := make([]byte, chunkSize)
			endOffset := fileInfo.Size() - chunkSize
			if endOffset < 0 {
				endOffset = 0
			}
			_, err = file.Seek(endOffset, 0)
			if err != nil {
				return nil, fmt.Errorf("failed to seek to end of file: %w", err)
			}
			n, err = file.Read(endBuffer)
			if err != nil && err != io.EOF {
				return nil, fmt.Errorf("failed to read end of file: %w", err)
			}
			endContent := string(endBuffer[:n])

			// Count total lines for context
			file.Seek(0, 0)
			scanner := bufio.NewScanner(file)
			lineCount := 0
			for scanner.Scan() {
				lineCount++
			}

			// Format the summary
			summary := fmt.Sprintf(`File: %s
Size: %d bytes (%.2f MB)
Lines: %d

WARNING: This file is too large to read entirely (exceeds %d bytes). Showing beginning and end portions only.
For detailed analysis of specific sections, please specify line ranges or search for specific content.

========== BEGINNING OF FILE (first %d bytes) ==========
%s

========== [TRUNCATED - %d bytes omitted] ==========

========== END OF FILE (last %d bytes) ==========
%s
========== END OF FILE ==========

To read specific sections, consider:
1. Using grep or search tools to find specific content
2. Reading the file in smaller chunks
3. Analyzing specific line ranges
4. Using specialized tools for the file type`,
				validPath,
				fileInfo.Size(), float64(fileInfo.Size())/(1024*1024),
				lineCount,
				maxFileSize,
				len(beginContent), beginContent,
				fileInfo.Size()-int64(len(beginContent)+len(endContent)),
				len(endContent), endContent)

			return summary, nil
		},
	)
}

func validatedReadFileChunkTool(projects []beau.ProjectBounds) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "read_file_chunk",
			Description: "Read a specific portion of a file by line numbers. Useful for reading large files in manageable chunks.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file to read (e.g., /home/user/project/file.txt)",
					},
					"start_line": map[string]interface{}{
						"type":        "integer",
						"description": "Starting line number (1-indexed). Default is 1.",
					},
					"end_line": map[string]interface{}{
						"type":        "integer",
						"description": "Ending line number (inclusive). If not specified, reads 1000 lines from start_line.",
					},
				},
				"required": []string{"file_path"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				FilePath  string `json:"file_path"`
				StartLine int    `json:"start_line"`
				EndLine   int    `json:"end_line"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Default values
			if args.StartLine <= 0 {
				args.StartLine = 1
			}
			if args.EndLine <= 0 {
				args.EndLine = args.StartLine + 999 // Default to 1000 lines
			}
			if args.EndLine < args.StartLine {
				return nil, fmt.Errorf("end_line must be greater than or equal to start_line")
			}

			// Validate path
			validPath, err := validatePath(projects, args.FilePath)
			if err != nil {
				return nil, err
			}

			// Open file
			file, err := os.Open(validPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open file '%s': %w", validPath, err)
			}
			defer file.Close()

			// Read specified lines
			scanner := bufio.NewScanner(file)
			var lines []string
			currentLine := 0
			totalLines := 0

			// First pass: count total lines
			for scanner.Scan() {
				totalLines++
			}

			// Reset to beginning
			file.Seek(0, 0)
			scanner = bufio.NewScanner(file)

			// Second pass: collect requested lines
			for scanner.Scan() {
				currentLine++
				if currentLine >= args.StartLine && currentLine <= args.EndLine {
					lines = append(lines, scanner.Text())
				}
				if currentLine > args.EndLine {
					break
				}
			}

			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("error reading file: %w", err)
			}

			if len(lines) == 0 {
				return nil, fmt.Errorf("no lines found in the specified range (file has %d lines)", totalLines)
			}

			// Format response
			actualEndLine := args.StartLine + len(lines) - 1
			header := fmt.Sprintf("File: %s\nLines %d-%d of %d:\n", validPath, args.StartLine, actualEndLine, totalLines)
			footer := ""

			if actualEndLine < totalLines {
				footer = fmt.Sprintf("\n\n[%d more lines remaining]", totalLines-actualEndLine)
			}

			return header + strings.Join(lines, "\n") + footer, nil
		},
	)
}

func validatedWriteFileTool(projects []beau.ProjectBounds) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "write_file",
			Description: "Write content to a file within the project directories",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file to write (e.g., /home/user/project/file.txt)",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Content to write to the file",
					},
				},
				"required": []string{"file_path", "content"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				FilePath string `json:"file_path"`
				Content  string `json:"content"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Validate path
			validPath, err := validatePath(projects, args.FilePath)
			if err != nil {
				return nil, err
			}

			// Create directory if needed
			dir := filepath.Dir(validPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory: %w", err)
			}

			// Write file
			err = os.WriteFile(validPath, []byte(args.Content), 0644)
			if err != nil {
				return nil, fmt.Errorf("failed to write file '%s': %w", validPath, err)
			}

			return fmt.Sprintf("Successfully wrote %d bytes to %s", len(args.Content), validPath), nil
		},
	)
}

func validatedListDirectoryTool(projects []beau.ProjectBounds) toolkit.LlmTool {
	// Build example paths from actual projects
	examplePath := "/home/user/project"
	exampleSubPath := "/home/user/project/subfolder"
	if len(projects) > 0 {
		examplePath = projects[0].ABSPath
		exampleSubPath = filepath.Join(projects[0].ABSPath, "subfolder")
	}

	description := "List contents of a directory within the project"
	if len(projects) > 0 {
		projectPaths := []string{}
		for _, p := range projects {
			projectPaths = append(projectPaths, fmt.Sprintf("%s (%s)", p.ABSPath, p.Name))
		}
		description = fmt.Sprintf("List contents of a directory within the project. Available directories: %s", strings.Join(projectPaths, ", "))
	}

	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "list_directory",
			Description: description,
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"directory_path": map[string]interface{}{
						"type":        "string",
						"description": fmt.Sprintf("Absolute path to directory (e.g., %s or %s)", examplePath, exampleSubPath),
					},
				},
				"required": []string{"directory_path"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				DirectoryPath string `json:"directory_path"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Default to project root
			if args.DirectoryPath == "" {
				args.DirectoryPath = "."
			}

			// Validate path
			validPath, err := validatePath(projects, args.DirectoryPath)
			if err != nil {
				return nil, err
			}

			// List directory
			entries, err := os.ReadDir(validPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read directory '%s': %w", validPath, err)
			}

			files := []map[string]interface{}{}
			directories := []map[string]interface{}{}

			for _, entry := range entries {
				info, err := entry.Info()
				if err != nil {
					continue
				}

				entryData := map[string]interface{}{
					"name":       entry.Name(),
					"size_bytes": info.Size(),
					"modified":   info.ModTime().Format("2006-01-02 15:04:05"),
				}

				if entry.IsDir() {
					directories = append(directories, entryData)
				} else {
					files = append(files, entryData)
				}
			}

			return map[string]interface{}{
				"path":        validPath,
				"directories": directories,
				"files":       files,
				"total_items": len(entries),
			}, nil
		},
	)
}

func validatedAnalyzeFileTool(projects []beau.ProjectBounds) toolkit.LlmTool {
	const maxFileSize = 400 * 1024 // Same limit as read_file tool

	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "analyze_file",
			Description: "Analyze a file to get size, modification time, line count, and recommendations for reading",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file to analyze (e.g., /home/user/project/file.txt)",
					},
				},
				"required": []string{"file_path"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				FilePath string `json:"file_path"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Validate path
			validPath, err := validatePath(projects, args.FilePath)
			if err != nil {
				return nil, err
			}

			// Get file info
			fileInfo, err := os.Stat(validPath)
			if err != nil {
				return nil, fmt.Errorf("failed to stat file '%s': %w", validPath, err)
			}

			if fileInfo.IsDir() {
				return nil, fmt.Errorf("path '%s' is a directory, not a file", validPath)
			}

			// Count lines (only for files under 10MB to avoid performance issues)
			lineCount := 0
			if fileInfo.Size() < 10*1024*1024 {
				content, err := os.ReadFile(validPath)
				if err == nil {
					lineCount = strings.Count(string(content), "\n") + 1
				}
			}

			result := map[string]interface{}{
				"file_path":   validPath,
				"size_bytes":  fileInfo.Size(),
				"size_human":  formatFileSize(fileInfo.Size()),
				"modified":    fileInfo.ModTime().Format("2006-01-02 15:04:05"),
				"permissions": fileInfo.Mode().String(),
			}

			if lineCount > 0 {
				result["line_count"] = lineCount
			}

			// Add recommendations based on file size
			if fileInfo.Size() > maxFileSize {
				result["warning"] = fmt.Sprintf("This file is too large to read entirely (%.2f MB). Use 'read_file_chunk' to read specific portions.", float64(fileInfo.Size())/(1024*1024))
				result["recommendation"] = "Use 'read_file_chunk' with start_line and end_line parameters to read specific sections"
			} else {
				result["can_read_fully"] = true
			}

			return result, nil
		},
	)
}

// Helper function to format file size in human-readable format
func formatFileSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}

func validatedRenameFileTool(projects []beau.ProjectBounds) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "rename_file",
			Description: "Rename or move a file within the project directories",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"old_path": map[string]interface{}{
						"type":        "string",
						"description": "Current absolute path of the file (e.g., /home/user/project/old.txt)",
					},
					"new_path": map[string]interface{}{
						"type":        "string",
						"description": "New absolute path for the file (e.g., /home/user/project/new.txt)",
					},
				},
				"required": []string{"old_path", "new_path"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				OldPath string `json:"old_path"`
				NewPath string `json:"new_path"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Validate old path
			validOldPath, err := validatePath(projects, args.OldPath)
			if err != nil {
				return nil, fmt.Errorf("old path validation failed: %w", err)
			}

			// Validate new path
			validNewPath, err := validatePath(projects, args.NewPath)
			if err != nil {
				return nil, fmt.Errorf("new path validation failed: %w", err)
			}

			// Check if source file exists
			if _, err := os.Stat(validOldPath); err != nil {
				if os.IsNotExist(err) {
					return nil, fmt.Errorf("source file '%s' does not exist", validOldPath)
				}
				return nil, fmt.Errorf("failed to stat source file '%s': %w", validOldPath, err)
			}

			// Check if destination already exists
			if _, err := os.Stat(validNewPath); err == nil {
				return nil, fmt.Errorf("destination file '%s' already exists", validNewPath)
			}

			// Create destination directory if needed
			destDir := filepath.Dir(validNewPath)
			if err := os.MkdirAll(destDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create destination directory: %w", err)
			}

			// Rename the file
			if err := os.Rename(validOldPath, validNewPath); err != nil {
				return nil, fmt.Errorf("failed to rename file from '%s' to '%s': %w", validOldPath, validNewPath, err)
			}

			return fmt.Sprintf("Successfully renamed file from '%s' to '%s'", validOldPath, validNewPath), nil
		},
	)
}

func validatedGrepFileTool(projects []beau.ProjectBounds) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "grep_file",
			Description: "Search for lines matching a pattern in a file. Returns matching lines with line numbers.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file to search (e.g., /home/user/project/file.txt)",
					},
					"pattern": map[string]interface{}{
						"type":        "string",
						"description": "Pattern to search for (supports simple string matching or regex)",
					},
					"use_regex": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to use regex matching (default: false)",
					},
					"ignore_case": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to ignore case when matching (default: false)",
					},
					"context_lines": map[string]interface{}{
						"type":        "integer",
						"description": "Number of context lines to show before and after matches (default: 0)",
					},
					"max_matches": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of matches to return (default: 100)",
					},
				},
				"required": []string{"file_path", "pattern"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				FilePath     string `json:"file_path"`
				Pattern      string `json:"pattern"`
				UseRegex     bool   `json:"use_regex"`
				IgnoreCase   bool   `json:"ignore_case"`
				ContextLines int    `json:"context_lines"`
				MaxMatches   int    `json:"max_matches"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Set defaults
			if args.MaxMatches <= 0 {
				args.MaxMatches = 100
			}
			if args.ContextLines < 0 {
				args.ContextLines = 0
			}

			// Validate path
			validPath, err := validatePath(projects, args.FilePath)
			if err != nil {
				return nil, err
			}

			// Open file
			file, err := os.Open(validPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open file '%s': %w", validPath, err)
			}
			defer file.Close()

			// Process pattern
			var matcher func(string) bool
			if args.UseRegex {
				// Compile regex
				regexPattern := args.Pattern
				if args.IgnoreCase {
					regexPattern = "(?i)" + regexPattern
				}
				re, err := regexp.Compile(regexPattern)
				if err != nil {
					return nil, fmt.Errorf("invalid regex pattern: %w", err)
				}
				matcher = func(line string) bool {
					return re.MatchString(line)
				}
			} else {
				// Simple string matching
				searchPattern := args.Pattern
				if args.IgnoreCase {
					searchPattern = strings.ToLower(searchPattern)
				}
				matcher = func(line string) bool {
					lineToCheck := line
					if args.IgnoreCase {
						lineToCheck = strings.ToLower(lineToCheck)
					}
					return strings.Contains(lineToCheck, searchPattern)
				}
			}

			// Read file and collect matches
			scanner := bufio.NewScanner(file)
			lineNum := 0
			matches := []map[string]interface{}{}
			allLines := []string{}

			// First pass: read all lines and find matches
			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				allLines = append(allLines, line)

				if matcher(line) && len(matches) < args.MaxMatches {
					matches = append(matches, map[string]interface{}{
						"line_number": lineNum,
						"line":        line,
						"index":       lineNum - 1, // 0-based index for later context retrieval
					})
				}
			}

			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("error reading file: %w", err)
			}

			if len(matches) == 0 {
				return map[string]interface{}{
					"file_path": validPath,
					"pattern":   args.Pattern,
					"matches":   []interface{}{},
					"message":   "No matches found",
				}, nil
			}

			// Add context lines if requested
			if args.ContextLines > 0 {
				for i, match := range matches {
					matchIndex := match["index"].(int)
					contextBefore := []string{}
					contextAfter := []string{}

					// Get context before
					for j := matchIndex - args.ContextLines; j < matchIndex; j++ {
						if j >= 0 {
							contextBefore = append(contextBefore, fmt.Sprintf("%d: %s", j+1, allLines[j]))
						}
					}

					// Get context after
					for j := matchIndex + 1; j <= matchIndex+args.ContextLines && j < len(allLines); j++ {
						contextAfter = append(contextAfter, fmt.Sprintf("%d: %s", j+1, allLines[j]))
					}

					matches[i]["context_before"] = contextBefore
					matches[i]["context_after"] = contextAfter
					delete(matches[i], "index") // Remove internal index from output
				}
			}

			result := map[string]interface{}{
				"file_path":     validPath,
				"pattern":       args.Pattern,
				"total_matches": len(matches),
				"matches":       matches,
			}

			if len(matches) == args.MaxMatches {
				result["warning"] = fmt.Sprintf("Results limited to %d matches", args.MaxMatches)
			}

			return result, nil
		},
	)
}

func validatedReplaceInFileTool(projects []beau.ProjectBounds) toolkit.LlmTool {
	return toolkit.NewTool(
		beau.ToolSchema{
			Name:        "replace_in_file",
			Description: "Replace all occurrences of a pattern in a file. Creates a backup before making changes.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"file_path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file to modify (e.g., /home/user/project/file.txt)",
					},
					"pattern": map[string]interface{}{
						"type":        "string",
						"description": "Pattern to search for (supports simple string matching or regex)",
					},
					"replacement": map[string]interface{}{
						"type":        "string",
						"description": "String to replace matches with",
					},
					"use_regex": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to use regex matching (default: false)",
					},
					"ignore_case": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to ignore case when matching (default: false)",
					},
					"create_backup": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to create a backup file before replacing (default: true)",
					},
					"dry_run": map[string]interface{}{
						"type":        "boolean",
						"description": "If true, only show what would be replaced without making changes (default: false)",
					},
				},
				"required": []string{"file_path", "pattern", "replacement"},
			},
		},
		func(input []byte) (interface{}, error) {
			var args struct {
				FilePath     string `json:"file_path"`
				Pattern      string `json:"pattern"`
				Replacement  string `json:"replacement"`
				UseRegex     bool   `json:"use_regex"`
				IgnoreCase   bool   `json:"ignore_case"`
				CreateBackup *bool  `json:"create_backup"`
				DryRun       bool   `json:"dry_run"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Default create_backup to true if not specified
			if args.CreateBackup == nil {
				createBackup := true
				args.CreateBackup = &createBackup
			}

			// Validate path
			validPath, err := validatePath(projects, args.FilePath)
			if err != nil {
				return nil, err
			}

			// Read file content
			content, err := os.ReadFile(validPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read file '%s': %w", validPath, err)
			}

			originalContent := string(content)
			replacedContent := originalContent
			replacementCount := 0

			if args.UseRegex {
				// Regex replacement
				regexPattern := args.Pattern
				if args.IgnoreCase {
					regexPattern = "(?i)" + regexPattern
				}
				re, err := regexp.Compile(regexPattern)
				if err != nil {
					return nil, fmt.Errorf("invalid regex pattern: %w", err)
				}

				// Count matches first
				matches := re.FindAllString(originalContent, -1)
				replacementCount = len(matches)

				// Perform replacement
				replacedContent = re.ReplaceAllString(originalContent, args.Replacement)
			} else {
				// Simple string replacement
				if args.IgnoreCase {
					// Case-insensitive string replacement requires more work
					lowerContent := strings.ToLower(originalContent)
					lowerPattern := strings.ToLower(args.Pattern)

					// Find all occurrences
					lastIndex := 0
					var result strings.Builder

					for {
						index := strings.Index(lowerContent[lastIndex:], lowerPattern)
						if index == -1 {
							result.WriteString(originalContent[lastIndex:])
							break
						}

						actualIndex := lastIndex + index
						result.WriteString(originalContent[lastIndex:actualIndex])
						result.WriteString(args.Replacement)
						lastIndex = actualIndex + len(args.Pattern)
						replacementCount++
					}

					replacedContent = result.String()
				} else {
					// Simple case-sensitive replacement
					replacementCount = strings.Count(originalContent, args.Pattern)
					replacedContent = strings.ReplaceAll(originalContent, args.Pattern, args.Replacement)
				}
			}

			// If dry run, just return what would happen
			if args.DryRun {
				return map[string]interface{}{
					"file_path":       validPath,
					"pattern":         args.Pattern,
					"replacement":     args.Replacement,
					"matches_found":   replacementCount,
					"dry_run":         true,
					"would_backup":    *args.CreateBackup,
					"size_before":     len(originalContent),
					"size_after":      len(replacedContent),
					"size_difference": len(replacedContent) - len(originalContent),
				}, nil
			}

			// No changes needed
			if replacementCount == 0 {
				return map[string]interface{}{
					"file_path":     validPath,
					"pattern":       args.Pattern,
					"replacement":   args.Replacement,
					"matches_found": 0,
					"message":       "No matches found, file unchanged",
				}, nil
			}

			// Create backup if requested
			var backupPath string
			if *args.CreateBackup {
				backupPath = validPath + ".bak"
				// Find a unique backup filename
				for i := 1; ; i++ {
					if _, err := os.Stat(backupPath); os.IsNotExist(err) {
						break
					}
					backupPath = fmt.Sprintf("%s.bak%d", validPath, i)
				}

				if err := os.WriteFile(backupPath, content, 0644); err != nil {
					return nil, fmt.Errorf("failed to create backup: %w", err)
				}
			}

			// Write the modified content
			if err := os.WriteFile(validPath, []byte(replacedContent), 0644); err != nil {
				// Try to restore from backup if write failed
				if backupPath != "" {
					os.Rename(backupPath, validPath)
				}
				return nil, fmt.Errorf("failed to write file: %w", err)
			}

			result := map[string]interface{}{
				"file_path":       validPath,
				"pattern":         args.Pattern,
				"replacement":     args.Replacement,
				"replacements":    replacementCount,
				"size_before":     len(originalContent),
				"size_after":      len(replacedContent),
				"size_difference": len(replacedContent) - len(originalContent),
			}

			if backupPath != "" {
				result["backup_path"] = backupPath
			}

			return result, nil
		},
	)
}
