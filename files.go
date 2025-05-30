package main

import (
	"fmt"
	"os"
)


// ValidateFilePath validates a file path exists and has a supported extension
func ValidateFilePath(path string, allowedTypes []string) error {
	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("file not found or not accessible: %w", err)
	}
	
	// Check if it's a regular file
	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", path)
	}
	
	// Check if file is too large
	if info.Size() > 10*1024*1024 { // 10MB limit by default
		return fmt.Errorf("file is too large: %s (%s)", path, humanReadableSize(info.Size()))
	}
	
	// Check file extension is allowed
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
