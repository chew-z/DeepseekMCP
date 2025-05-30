package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	_ "github.com/joho/godotenv/autoload"
	mcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// main is the entry point for the application.
// It sets up the MCP server with the appropriate handlers and starts it.
func main() {
	// Define command-line flags for configuration override
	deepseekModelFlag := flag.String("deepseek-model", "", "DeepSeek model name (overrides env var)")
	deepseekSystemPromptFlag := flag.String("deepseek-system-prompt", "", "System prompt (overrides env var)")
	deepseekTemperatureFlag := flag.Float64("deepseek-temperature", -1, "Temperature setting (0.0-1.0, overrides env var)")
	flag.Parse()

	// Create application context with logger
	logger := NewLogger(LevelInfo)
	ctx := context.WithValue(context.Background(), loggerKey, logger)

	// Create configuration from environment variables
	config, err := NewConfig()
	if err != nil {
		handleStartupError(ctx, err)
		return
	}

	// Override with command-line flags if provided
	// Model ID validation will happen after deepseekServer is initialized
	if *deepseekModelFlag != "" {
		logger.Info("Overriding DeepSeek model with flag value: %s", *deepseekModelFlag)
		config.DeepseekModel = *deepseekModelFlag
	}
	if *deepseekSystemPromptFlag != "" {
		logger.Info("Overriding DeepSeek system prompt with flag value")
		config.DeepseekSystemPrompt = *deepseekSystemPromptFlag
	}

	// Override temperature if provided and valid
	if *deepseekTemperatureFlag >= 0 {
		// Validate temperature is within range
		if *deepseekTemperatureFlag > 1.0 {
			logger.Error("Invalid temperature value: %v. Must be between 0.0 and 1.0", *deepseekTemperatureFlag)
			handleStartupError(ctx, fmt.Errorf("invalid temperature: %v", *deepseekTemperatureFlag))
			return
		}
		logger.Info("Overriding DeepSeek temperature with flag value: %v", *deepseekTemperatureFlag)
		config.DeepseekTemperature = float32(*deepseekTemperatureFlag)
	}

	// Store config in context for error handler to access
	ctx = context.WithValue(ctx, configKey, config)

	// Set up handler registry
	// NewHandlerRegistry is a constructor that doesn't return an error

	// Create the MCP server instance
	srv := server.NewMCPServer("deepseek", "1.0.0")

	// Create and register the DeepSeek server (now passing the created srv)
	deepseekServer, err := setupDeepseekServer(ctx, srv, config)
	if err != nil {
		handleStartupError(ctx, err) // handleStartupError will also use an MCPServer now
		return
	}

	// Validate the effective model ID (from config, possibly overridden by flag)
	if err := deepseekServer.ValidateModelID(config.DeepseekModel); err != nil {
		logger.Error("Effective model ID validation failed: %v", err)
		// Use a more specific error message for startup failure
		startupErr := fmt.Errorf("effective model ID \"%s\" is invalid: %w", config.DeepseekModel, err)
		logger.Error("Startup error: %v", startupErr) // Log the specific startup error
		handleStartupError(ctx, startupErr) // Pass the specific startup error
		return
	}

	// Start the MCP server
	logger.Info("Starting DeepSeek MCP server via Stdio")
	if err := server.ServeStdio(srv); err != nil {
		logger.Error("Server error: %v", err)
		os.Exit(1)
	}
}

// setupDeepseekServer creates and registers a DeepSeek server
func setupDeepseekServer(ctx context.Context, srv *server.MCPServer, config *Config) (*DeepseekServer, error) {
	loggerValue := ctx.Value(loggerKey)
	logger, ok := loggerValue.(Logger)
	if !ok {
		return nil, fmt.Errorf("logger not found in context")
	}

	// Create the DeepSeek server with configuration
	deepseekServer, err := NewDeepseekServer(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create DeepSeek server: %w", err)
	}

	// Wrap the server with logger middleware

	// Register the wrapped server
	// Define and register tools
	askTool := mcp.NewTool("deepseek_ask",
		mcp.WithDescription("Use DeepSeek's AI model to ask about complex coding problems"),
		mcp.AddParameter(mcp.StringParameter("query", mcp.Required(), mcp.Description("The coding problem or question for DeepSeek AI, including any relevant code."))),
		mcp.AddParameter(mcp.StringParameter("model", mcp.Description("Optional: Specific DeepSeek model to use (e.g., deepseek-chat, deepseek-coder). Overrides default configuration."))),
		mcp.AddParameter(mcp.StringParameter("systemPrompt", mcp.Description("Optional: Custom system prompt to guide the AI's behavior for this request. Overrides default configuration."))),
		mcp.AddParameter(mcp.StringArrayParameter("file_paths", mcp.Description("Optional: Paths to files to include in the request context. Content will be appended to the query."))),
		mcp.AddParameter(mcp.BoolParameter("json_mode", mcp.Description("Optional: Enable JSON mode for structured JSON responses. Set to true when expecting JSON output."))),
	)
	srv.AddTool(askTool, deepseekServer.handleAskDeepseek)

	modelsTool := mcp.NewTool("deepseek_models",
		mcp.WithDescription("List available DeepSeek models with descriptions"),
	)
	srv.AddTool(modelsTool, deepseekServer.handleDeepseekModels)

	balanceTool := mcp.NewTool("deepseek_balance",
		mcp.WithDescription("Check your DeepSeek API account balance"),
	)
	srv.AddTool(balanceTool, deepseekServer.handleDeepseekBalance)

	tokenEstimateTool := mcp.NewTool("deepseek_token_estimate",
		mcp.WithDescription("Estimate the number of tokens in a given text or file content."),
		mcp.AddParameter(mcp.StringParameter("text", mcp.Description("Text to estimate token count for. Use this or file_path."))),
		mcp.AddParameter(mcp.StringParameter("file_path", mcp.Description("Path to a file to estimate token count for. Use this or text."))),
	)
	srv.AddTool(tokenEstimateTool, deepseekServer.handleTokenEstimate)

	logger.Info("Registered DeepSeek tools and server in normal mode with model: %s", config.DeepseekModel) // Updated log message

	// Log file handling configuration
	logger.Info("File handling: max size %s, allowed types: %v",
		humanReadableSize(config.MaxFileSize),
		config.AllowedFileTypes)

	// Log a truncated version of the system prompt for security/brevity
	promptPreview := config.DeepseekSystemPrompt
	if len(promptPreview) > 50 {
		// Use proper UTF-8 safe truncation
		runeCount := 0
		for i := range promptPreview {
			runeCount++
			if runeCount > 50 {
				promptPreview = promptPreview[:i] + "..."
				break
			}
		}
	}
	logger.Info("Using system prompt: %s", promptPreview)

	return deepseekServer, nil
}

// handleStartupError handles initialization errors by setting up an error server
func handleStartupError(ctx context.Context, err error) {
	// Safely extract logger from context
	loggerValue := ctx.Value(loggerKey)
	logger, ok := loggerValue.(Logger)
	if !ok {
		// Fallback to a new logger if type assertion fails
		logger = NewLogger(LevelError)
	}
	errorMsg := err.Error()

	logger.Error("Initialization error: %v", err)

	// Get config for EnableCaching status (if available)
	var config *Config
	configValue := ctx.Value(configKey)
	if configValue != nil {
		if cfg, ok := configValue.(*Config); ok {
			config = cfg
		}
	}

	// Create error server (This block is removed)
	// errorServer := &ErrorDeepseekServer{
	// 	errorMessage: errorMsg,
	// 	config:       config,
	// }

	// Set up registry with error server
	// NewHandlerRegistry is a constructor that doesn't return an error
	// errorServerWithLogger := NewLoggerMiddleware(errorServer, logger) // Middleware removed for now
	// registry.RegisterToolHandler(errorServerWithLogger) // Registry removed

	// Start server in degraded mode
	logger.Info("Starting DeepSeek MCP server in degraded mode via Stdio")
	errorSrv := server.NewMCPServer("deepseek-error", "1.0.0")

	// Define a specific error tool and handler
	errorTool := mcp.NewTool("startup_error", mcp.WithDescription("Provides server startup error information"))
	placeholderErrorHandler := func(toolCtx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Use errorMsg directly from the outer function's scope
		return mcp.NewToolResultText(errorMsg), nil
	}
	errorSrv.AddTool(errorTool, placeholderErrorHandler)

	if err := server.ServeStdio(errorSrv); err != nil {
		logger.Error("Server error in degraded mode: %v", err)
		os.Exit(1)
	}
}
