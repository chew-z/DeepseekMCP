package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateFilePath validates a file path exists and conforms to the
// constraints defined in the provided Config (max size and allowed types).
// If cfg is nil, a 10MB default max size is used and types are not restricted.
func ValidateFilePath(path string, cfg *Config) error {
	// First, check if the path is in the allowed list of directories
	if cfg != nil && len(cfg.AllowedFilePaths) > 0 {
		if !isPathAllowed(path, cfg.AllowedFilePaths) {
			return fmt.Errorf("file path is not allowed: %s. Allowed roots are: %s", path, strings.Join(cfg.AllowedFilePaths, ", "))
		}
	}

	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("file not found or not accessible: %w", err)
	}

	// Check if it's a regular file
	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", path)
	}

	// Determine max file size from config (default 10MB if cfg is nil)
	var maxSize int64 = 10 * 1024 * 1024
	if cfg != nil && cfg.MaxFileSize > 0 {
		maxSize = cfg.MaxFileSize
	}

	// Check if file is too large
	if info.Size() > maxSize {
		return fmt.Errorf("file is too large: %s (%s)", path, humanReadableSize(info.Size()))
	}

	// Check file extension is allowed
	var allowedTypes []string
	if cfg != nil {
		allowedTypes = cfg.AllowedFileTypes
	}
	if len(allowedTypes) > 0 {
		mimeType := getMimeTypeFromPath(path)
		allowed := false
		for _, allowedType := range allowedTypes {
			if mimeType == allowedType {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("file type not allowed: %s (type: %s)", path, mimeType)
		}
	}

	return nil
}

// GetFileInfo returns information about a file
func GetFileInfo(path string) (string, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}

	mimeType := getMimeTypeFromPath(path)
	return mimeType, info.Size(), nil
}

// isPathAllowed checks if a given file path is within the allowed directories.
// This is a security measure to prevent arbitrary file system access.
func isPathAllowed(path string, allowedDirs []string) bool {
	// Resolve the target path to an absolute, symlink-free path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If we cannot resolve symlinks on the target path, deny access
		return false
	}

	for _, dir := range allowedDirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}

		// Resolve each allowed directory to an absolute, symlink-free path
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		resolvedDir, err := filepath.EvalSymlinks(absDir)
		if err != nil {
			continue
		}

		// Compute the relative path from the allowed directory to the target
		rel, err := filepath.Rel(resolvedDir, resolvedPath)
		if err != nil {
			continue
		}

		rel = filepath.Clean(rel)

		// Allowed if the relative path does not traverse outside the dir
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
			return true
		}
	}
	return false
}
