package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bosley/beau"
)

// ValidatePath checks if a path is within allowed project directories
func ValidatePath(projects []beau.ProjectBounds, path string) (string, error) {
	// Handle empty path as current directory
	if path == "" {
		path = "."
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("could not get absolute path for '%s': %w", path, err)
	}

	// If no project bounds specified, allow any path
	if len(projects) == 0 {
		return absPath, nil
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
