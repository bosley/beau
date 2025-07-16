package fskit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bosley/beau"
)

// Helper function to create a test project bounds
func createTestProjectBounds(t *testing.T) ([]beau.ProjectBounds, string) {
	tempDir, err := os.MkdirTemp("", "fskit_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	projects := []beau.ProjectBounds{
		{
			Name:        "test_project",
			Description: "Test project for fskit",
			ABSPath:     tempDir,
		},
	}

	return projects, tempDir
}

// Helper function to create a test file with content
func createTestFile(t *testing.T, dir, filename, content string) string {
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	return filePath
}

// Helper callback for testing
func testCallback(isError bool, id string, result interface{}) {
	// Empty callback for testing
}

func TestValidatedGrepFileTool(t *testing.T) {
	projects, tempDir := createTestProjectBounds(t)
	defer os.RemoveAll(tempDir)

	// Create test file with content
	testContent := `Line 1: Hello World
Line 2: This is a test file
Line 3: Hello again
Line 4: Testing grep functionality
Line 5: HELLO in uppercase
Line 6: Another test line
Line 7: Final line with Hello`

	testFile := createTestFile(t, tempDir, "test.txt", testContent)

	// Create the grep tool
	grepTool := validatedGrepFileTool(projects)

	tests := []struct {
		name           string
		args           map[string]interface{}
		expectError    bool
		validateResult func(t *testing.T, result interface{})
	}{
		{
			name: "Simple string search",
			args: map[string]interface{}{
				"file_path": testFile,
				"pattern":   "Hello",
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				res := result.(map[string]interface{})
				matches := res["matches"].([]map[string]interface{})
				if len(matches) != 3 {
					t.Errorf("Expected 3 matches, got %d", len(matches))
				}
				// Check first match
				if matches[0]["line_number"].(int) != 1 {
					t.Errorf("Expected first match on line 1")
				}
			},
		},
		{
			name: "Case insensitive search",
			args: map[string]interface{}{
				"file_path":   testFile,
				"pattern":     "hello",
				"ignore_case": true,
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				res := result.(map[string]interface{})
				matches := res["matches"].([]map[string]interface{})
				if len(matches) != 4 { // Should match "Hello" and "HELLO"
					t.Errorf("Expected 4 matches with case insensitive, got %d", len(matches))
				}
			},
		},
		{
			name: "Regex search",
			args: map[string]interface{}{
				"file_path": testFile,
				"pattern":   "Line \\d+:",
				"use_regex": true,
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				res := result.(map[string]interface{})
				matches := res["matches"].([]map[string]interface{})
				if len(matches) != 7 { // All lines have "Line X:" pattern
					t.Errorf("Expected 7 matches, got %d", len(matches))
				}
			},
		},
		{
			name: "Search with context lines",
			args: map[string]interface{}{
				"file_path":     testFile,
				"pattern":       "grep",
				"context_lines": 1,
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				res := result.(map[string]interface{})
				matches := res["matches"].([]map[string]interface{})
				if len(matches) != 1 {
					t.Errorf("Expected 1 match, got %d", len(matches))
				}
				match := matches[0]
				contextBefore := match["context_before"].([]string)
				contextAfter := match["context_after"].([]string)
				if len(contextBefore) != 1 || len(contextAfter) != 1 {
					t.Errorf("Expected 1 context line before and after")
				}
			},
		},
		{
			name: "No matches found",
			args: map[string]interface{}{
				"file_path": testFile,
				"pattern":   "NotFound",
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				res := result.(map[string]interface{})
				matches := res["matches"].([]interface{})
				if len(matches) != 0 {
					t.Errorf("Expected 0 matches, got %d", len(matches))
				}
				if res["message"] != "No matches found" {
					t.Errorf("Expected 'No matches found' message")
				}
			},
		},
		{
			name: "Invalid file path",
			args: map[string]interface{}{
				"file_path": "/invalid/path/file.txt",
				"pattern":   "test",
			},
			expectError: true,
		},
		{
			name: "Max matches limit",
			args: map[string]interface{}{
				"file_path":   testFile,
				"pattern":     "Line",
				"max_matches": 2,
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				res := result.(map[string]interface{})
				matches := res["matches"].([]map[string]interface{})
				if len(matches) != 2 {
					t.Errorf("Expected 2 matches (limited), got %d", len(matches))
				}
				if _, hasWarning := res["warning"]; !hasWarning {
					t.Errorf("Expected warning about limited results")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal args to JSON
			argsJSON, err := json.Marshal(tt.args)
			if err != nil {
				t.Fatalf("Failed to marshal args: %v", err)
			}

			// Call the tool
			result, err := grepTool.Call(argsJSON)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}
		})
	}
}

func TestValidatedReplaceInFileTool(t *testing.T) {
	projects, tempDir := createTestProjectBounds(t)
	defer os.RemoveAll(tempDir)

	// Create the replace tool
	replaceTool := validatedReplaceInFileTool(projects)

	tests := []struct {
		name           string
		initialContent string
		args           map[string]interface{}
		expectError    bool
		validateResult func(t *testing.T, result interface{}, filePath string)
	}{
		{
			name: "Simple string replacement",
			initialContent: `Hello World
This is a test
Hello again`,
			args: map[string]interface{}{
				"pattern":     "Hello",
				"replacement": "Hi",
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}, filePath string) {
				res := result.(map[string]interface{})
				if res["replacements"].(int) != 2 {
					t.Errorf("Expected 2 replacements, got %v", res["replacements"])
				}
				// Check file content
				content, _ := os.ReadFile(filePath)
				if !strings.Contains(string(content), "Hi World") {
					t.Errorf("Expected 'Hi World' in file content")
				}
			},
		},
		{
			name: "Case insensitive replacement",
			initialContent: `Hello World
hello there
HELLO again`,
			args: map[string]interface{}{
				"pattern":     "hello",
				"replacement": "Greetings",
				"ignore_case": true,
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}, filePath string) {
				res := result.(map[string]interface{})
				if res["replacements"].(int) != 3 {
					t.Errorf("Expected 3 replacements, got %v", res["replacements"])
				}
				content, _ := os.ReadFile(filePath)
				if strings.Contains(string(content), "Hello") || strings.Contains(string(content), "hello") {
					t.Errorf("Old pattern should not exist after replacement")
				}
			},
		},
		{
			name: "Regex replacement",
			initialContent: `Phone: 123-456-7890
Call: 555-123-4567
Fax: 999-888-7777`,
			args: map[string]interface{}{
				"pattern":     "\\d{3}-\\d{3}-\\d{4}",
				"replacement": "XXX-XXX-XXXX",
				"use_regex":   true,
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}, filePath string) {
				res := result.(map[string]interface{})
				if res["replacements"].(int) != 3 {
					t.Errorf("Expected 3 replacements, got %v", res["replacements"])
				}
				content, _ := os.ReadFile(filePath)
				if !strings.Contains(string(content), "XXX-XXX-XXXX") {
					t.Errorf("Expected masked phone numbers")
				}
			},
		},
		{
			name: "Dry run mode",
			initialContent: `Test content
Will not be changed`,
			args: map[string]interface{}{
				"pattern":     "Test",
				"replacement": "Demo",
				"dry_run":     true,
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}, filePath string) {
				res := result.(map[string]interface{})
				if res["dry_run"] != true {
					t.Errorf("Expected dry_run to be true")
				}
				if res["matches_found"].(int) != 1 {
					t.Errorf("Expected 1 match in dry run")
				}
				// Check file wasn't modified
				content, _ := os.ReadFile(filePath)
				if !strings.Contains(string(content), "Test content") {
					t.Errorf("File should not be modified in dry run")
				}
			},
		},
		{
			name:           "No matches found",
			initialContent: `Some content here`,
			args: map[string]interface{}{
				"pattern":     "NotFound",
				"replacement": "Replaced",
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}, filePath string) {
				res := result.(map[string]interface{})
				if res["matches_found"].(int) != 0 {
					t.Errorf("Expected 0 matches")
				}
				if res["message"] != "No matches found, file unchanged" {
					t.Errorf("Expected no matches message")
				}
			},
		},
		{
			name:           "Backup creation",
			initialContent: `Original content`,
			args: map[string]interface{}{
				"pattern":       "Original",
				"replacement":   "Modified",
				"create_backup": true,
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}, filePath string) {
				res := result.(map[string]interface{})
				backupPath, hasBackup := res["backup_path"].(string)
				if !hasBackup {
					t.Errorf("Expected backup_path in result")
				}
				// Check backup exists
				if _, err := os.Stat(backupPath); err != nil {
					t.Errorf("Backup file should exist: %v", err)
				}
				// Check backup content
				backupContent, _ := os.ReadFile(backupPath)
				if !strings.Contains(string(backupContent), "Original content") {
					t.Errorf("Backup should contain original content")
				}
				// Clean up backup
				os.Remove(backupPath)
			},
		},
		{
			name:           "No backup creation",
			initialContent: `Test content`,
			args: map[string]interface{}{
				"pattern":       "Test",
				"replacement":   "Demo",
				"create_backup": false,
			},
			expectError: false,
			validateResult: func(t *testing.T, result interface{}, filePath string) {
				res := result.(map[string]interface{})
				if _, hasBackup := res["backup_path"]; hasBackup {
					t.Errorf("Should not create backup when create_backup is false")
				}
			},
		},
		{
			name:           "Invalid file path",
			initialContent: "",
			args: map[string]interface{}{
				"file_path":   "/invalid/path/file.txt",
				"pattern":     "test",
				"replacement": "demo",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file for each test
			testFile := ""
			if tt.initialContent != "" {
				testFile = createTestFile(t, tempDir, tt.name+".txt", tt.initialContent)
				if _, hasPath := tt.args["file_path"]; !hasPath {
					tt.args["file_path"] = testFile
				}
			}

			// Marshal args to JSON
			argsJSON, err := json.Marshal(tt.args)
			if err != nil {
				t.Fatalf("Failed to marshal args: %v", err)
			}

			// Call the tool
			result, err := replaceTool.Call(argsJSON)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else if tt.validateResult != nil {
					tt.validateResult(t, result, testFile)
				}
			}
		})
	}
}
