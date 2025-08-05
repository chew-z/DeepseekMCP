package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the configuration for the DeepseekMCP server
type Config struct {
	// API configuration
	DeepseekAPIKey       string
	DeepseekModel        string
	DeepseekSystemPrompt string
	MaxFileSize          int64
	AllowedFileTypes     []string
	DeepseekTemperature  float32
	HTTPTimeout          time.Duration
	MaxRetries           int
	InitialBackoff       time.Duration
	MaxBackoff           time.Duration
	AllowedFilePaths     []string // New field for allowed file paths
}

// NewConfig creates a new configuration instance from environment variables
func NewConfig() (*Config, error) {
	// Read API key (required)
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		return nil, errors.New("DEEPSEEK_API_KEY environment variable is required")
	}

	// Read model (optional, defaults to "deepseek-reasoner")
	model := os.Getenv("DEEPSEEK_MODEL")
	if model == "" {
		model = "deepseek-reasoner"
	}

	// Read system prompt (optional)
	systemPrompt := os.Getenv("DEEPSEEK_SYSTEM_PROMPT")
	if systemPrompt == "" {
		// Load from file if provided
		systemPromptPath := os.Getenv("DEEPSEEK_SYSTEM_PROMPT_FILE")
		if systemPromptPath != "" {
			data, err := os.ReadFile(systemPromptPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read system prompt file: %w", err)
			}
			systemPrompt = string(data)
		} else {
			// Default system prompt for code review
			systemPrompt = "You are a helpful AI assistant that specializes in code review and software engineering. " +
				"Provide thorough and insightful analysis with specific, actionable feedback. " +
				"Focus on issues like bugs, security vulnerabilities, performance problems, and code quality. " +
				"Include examples and explanations in your reviews."
		}
	}

	// Read max file size (optional, defaults to 10MB)
	maxFileSizeStr := os.Getenv("DEEPSEEK_MAX_FILE_SIZE")
	var maxFileSize int64 = 10 * 1024 * 1024 // 10MB default
	if maxFileSizeStr != "" {
		var err error
		maxFileSize, err = strconv.ParseInt(maxFileSizeStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid DEEPSEEK_MAX_FILE_SIZE: %w", err)
		}
	}

	// Read allowed file types (optional, defaults to common code file types)
	allowedFileTypesStr := os.Getenv("DEEPSEEK_ALLOWED_FILE_TYPES")
	var allowedFileTypes []string
	if allowedFileTypesStr == "" {
		// Default allowed file types
		allowedFileTypes = []string{
			"text/plain", "text/x-go", "text/x-python", "text/javascript",
			"text/markdown", "text/x-java", "text/x-c", "text/x-c++",
			"text/csv", "application/json", "text/x-yaml", "text/x-toml",
			"text/html", "text/css", "application/xml",
		}
	} else {
		allowedFileTypes = strings.Split(allowedFileTypesStr, ",")
	}

	// Read temperature (optional, defaults to 0.4)
	tempStr := os.Getenv("DEEPSEEK_TEMPERATURE")
	var temperature float32 = 0.4
	if tempStr != "" {
		tempFloat, err := strconv.ParseFloat(tempStr, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid DEEPSEEK_TEMPERATURE: %w", err)
		}
		temperature = float32(tempFloat)
	}

	// Read HTTP timeout (optional, defaults to 90 seconds)
	timeoutStr := os.Getenv("DEEPSEEK_TIMEOUT")
	timeout := 90 * time.Second
	if timeoutStr != "" {
		// if no unit is provided, assume seconds
		if _, err := strconv.Atoi(timeoutStr); err == nil {
			timeoutStr += "s"
		}
		var err error
		timeout, err = time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid DEEPSEEK_TIMEOUT: %w", err)
		}
	}

	// Read max retries (optional, defaults to 2)
	maxRetriesStr := os.Getenv("DEEPSEEK_MAX_RETRIES")
	maxRetries := 2
	if maxRetriesStr != "" {
		var err error
		maxRetries, err = strconv.Atoi(maxRetriesStr)
		if err != nil {
			return nil, fmt.Errorf("invalid DEEPSEEK_MAX_RETRIES: %w", err)
		}
	}

	// Read initial backoff (optional, defaults to 1 second)
	initialBackoffStr := os.Getenv("DEEPSEEK_INITIAL_BACKOFF")
	initialBackoff := 1 * time.Second
	if initialBackoffStr != "" {
		var err error
		initialBackoff, err = time.ParseDuration(initialBackoffStr)
		if err != nil {
			return nil, fmt.Errorf("invalid DEEPSEEK_INITIAL_BACKOFF: %w", err)
		}
	}

	// Read max backoff (optional, defaults to 10 seconds)
	maxBackoffStr := os.Getenv("DEEPSEEK_MAX_BACKOFF")
	maxBackoff := 10 * time.Second
	if maxBackoffStr != "" {
		var err error
		maxBackoff, err = time.ParseDuration(maxBackoffStr)
		if err != nil {
			return nil, fmt.Errorf("invalid DEEPSEEK_MAX_BACKOFF: %w", err)
		}
	}

	// Read allowed file paths (optional, defaults to current working directory)
	allowedFilePathsStr := os.Getenv("DEEPSEEK_ALLOWED_FILE_PATHS")
	var allowedFilePaths []string
	if allowedFilePathsStr == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current working directory: %w", err)
		}
		allowedFilePaths = []string{wd}
	} else {
		allowedFilePaths = strings.Split(allowedFilePathsStr, ",")
	}

	return &Config{
		DeepseekAPIKey:       apiKey,
		DeepseekModel:        model,
		DeepseekSystemPrompt: systemPrompt,
		MaxFileSize:          maxFileSize,
		AllowedFileTypes:     allowedFileTypes,
		DeepseekTemperature:  temperature,
		HTTPTimeout:          timeout,
		MaxRetries:           maxRetries,
		InitialBackoff:       initialBackoff,
		MaxBackoff:           maxBackoff,
		AllowedFilePaths:     allowedFilePaths,
	}, nil
}
