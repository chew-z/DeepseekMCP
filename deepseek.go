package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	
	"github.com/cohesion-org/deepseek-go"
	"github.com/gomcpgo/mcp/pkg/protocol"
)

// DeepseekServer implements the ToolHandler interface for DeepSeek API interactions
type DeepseekServer struct {
	config  *Config
	client  *deepseek.Client
	models  []DeepseekModelInfo   // Dynamically discovered models
	modelsMu sync.RWMutex         // Mutex for thread-safe model access
}



// NewDeepseekServer creates a new DeepseekServer with the provided configuration
func NewDeepseekServer(ctx context.Context, config *Config) (*DeepseekServer, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	if config.DeepseekAPIKey == "" {
		return nil, errors.New("DeepSeek API key is required")
	}

	// Initialize the DeepSeek client
	client := deepseek.NewClient(config.DeepseekAPIKey)
	
	// No error is returned by NewClient in the current library version

	// Create a simplified DeepseekServer without cache storage
	server := &DeepseekServer{
		config: config,
		client: client,
	}
	
	// Discover available models at startup
	err := server.discoverModels(ctx)
	if err != nil {
		// Log warning but continue - we'll use fallback models if needed
		logger := getLoggerFromContext(ctx)
		logger.Warn("Failed to discover DeepSeek models, will use fallback models: %v", err)
	}
	
	return server, nil
}

// Close closes the DeepSeek client connection (not needed for the DeepSeek API)
func (s *DeepseekServer) Close() {
	// No need to close the client in the DeepSeek API
}

// discoverModels fetches the available models from the DeepSeek API
func (s *DeepseekServer) discoverModels(ctx context.Context) error {
	logger := getLoggerFromContext(ctx)
	logger.Info("Discovering available DeepSeek models from API")
	
	// Get models from the API
	apiModels, err := deepseek.ListAllModels(s.client, ctx)
	if err != nil {
		logger.Error("Failed to get models from DeepSeek API: %v", err)
		return err
	}
	
	// Convert to our internal model format
	var models []DeepseekModelInfo
	for _, apiModel := range apiModels.Data {
		modelName := s.formatModelName(apiModel.ID)
		
		models = append(models, DeepseekModelInfo{
			ID:          apiModel.ID,
			Name:        modelName,
			Description: fmt.Sprintf("Model provided by %s", apiModel.OwnedBy),
		})
	}
	
	// Update the models list with thread safety
	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()
	s.models = models
	
	logger.Info("Discovered %d DeepSeek models", len(models))
	return nil
}

// formatModelName converts API model IDs to human-readable names
func (s *DeepseekServer) formatModelName(modelID string) string {
	// Replace hyphens with spaces and capitalize words
	parts := strings.Split(modelID, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	
	return strings.Join(parts, " ")
}

// ListTools implements the ToolHandler interface for DeepseekServer
func (s *DeepseekServer) ListTools(ctx context.Context) (*protocol.ListToolsResponse, error) {
	tools := []protocol.Tool{
		{
			Name:        "deepseek_ask",
			Description: "Use DeepSeek's AI model to ask about complex coding problems",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "The coding problem that we are asking DeepSeek AI to work on [question + code]"
					},
					"model": {
						"type": "string",
						"description": "Optional: Specific DeepSeek model to use (overrides default configuration)"
					},
					"systemPrompt": {
						"type": "string",
						"description": "Optional: Custom system prompt to use for this request (overrides default configuration)"
					},
					"file_paths": {
						"type": "array",
						"items": {
							"type": "string"
						},
						"description": "Optional: Paths to files to include in the request context"
					},
					"json_mode": {
						"type": "boolean",
						"description": "Optional: Enable JSON mode to receive structured JSON responses. Set to true when you expect JSON output."
					}
				},
				"required": ["query"]
			}`),
		},
		{
			Name:        "deepseek_models",
			Description: "List available DeepSeek models with descriptions",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {},
				"required": []
			}`),
		},
		{
			Name:        "deepseek_balance",
			Description: "Check your DeepSeek API account balance",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {},
				"required": []
			}`),
		},
		{
			Name:        "deepseek_token_estimate",
			Description: "Estimate the number of tokens in text or a file",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"text": {
						"type": "string",
						"description": "Text to estimate token count for"
					},
					"file_path": {
						"type": "string",
						"description": "Path to file to estimate token count for"
					}
				},
				"required": []
			}`),
		},
	}

	return &protocol.ListToolsResponse{
		Tools: tools,
	}, nil
}

// getLoggerFromContext safely extracts a logger from the context or creates a new one
func getLoggerFromContext(ctx context.Context) Logger {
	loggerValue := ctx.Value(loggerKey)
	if loggerValue != nil {
		if l, ok := loggerValue.(Logger); ok {
			return l
		}
	}
	// Create a new logger if one isn't in the context or type assertion fails
	return NewLogger(LevelInfo)
}

// createErrorResponse creates a standardized error response
func createErrorResponse(message string) *protocol.CallToolResponse {
	return &protocol.CallToolResponse{
		IsError: true,
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: message,
			},
		},
	}
}

// CallTool implements the ToolHandler interface for DeepseekServer
func (s *DeepseekServer) CallTool(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	switch req.Name {
	case "deepseek_ask":
		return s.handleAskDeepseek(ctx, req)
	case "deepseek_models":
		return s.handleDeepseekModels(ctx)
	case "deepseek_balance":
		return s.handleDeepseekBalance(ctx)
	case "deepseek_token_estimate":
		return s.handleTokenEstimate(ctx, req)
	default:
		return createErrorResponse(fmt.Sprintf("unknown tool: %s", req.Name)), nil
	}
}

// handleAskDeepseek handles requests to the ask_deepseek tool
func (s *DeepseekServer) handleAskDeepseek(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)

	// Extract and validate query parameter (required)
	query, ok := req.Arguments["query"].(string)
	if !ok {
		return createErrorResponse("query must be a string"), nil
	}

	// Extract optional model parameter
	modelName := s.config.DeepseekModel
	if customModel, ok := req.Arguments["model"].(string); ok && customModel != "" {
		// Validate the custom model
		if err := s.ValidateModelID(customModel); err != nil {
			logger.Error("Invalid model requested: %v", err)
			return createErrorResponse(fmt.Sprintf("Invalid model specified: %v", err)), nil
		}
		logger.Info("Using request-specific model: %s", customModel)
		modelName = customModel
	}

	// Extract optional systemPrompt parameter
	systemPrompt := s.config.DeepseekSystemPrompt
	if customPrompt, ok := req.Arguments["systemPrompt"].(string); ok && customPrompt != "" {
		logger.Info("Using request-specific system prompt")
		systemPrompt = customPrompt
	}

	// Extract file paths if provided
	var filePaths []string
	if filePathsRaw, ok := req.Arguments["file_paths"].([]interface{}); ok {
		for _, pathRaw := range filePathsRaw {
			if path, ok := pathRaw.(string); ok {
				filePaths = append(filePaths, path)
			}
		}
	}

	// Extract optional JSON mode parameter
	jsonMode := false
	if jsonModeRaw, ok := req.Arguments["json_mode"].(bool); ok {
		jsonMode = jsonModeRaw
		logger.Info("JSON mode is enabled: %v", jsonMode)
	}


	// Create ChatCompletionMessage from user query and system prompt
	chatMessages := []deepseek.ChatCompletionMessage{
		{
			Role:    deepseek.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    deepseek.ChatMessageRoleUser,
			Content: query,
		},
	}

	// Create the request
	request := &deepseek.ChatCompletionRequest{
		Model:       modelName,
		Messages:    chatMessages,
		Temperature: s.config.DeepseekTemperature,
		JSONMode:    jsonMode,
	}

	// Log the temperature setting
	logger.Debug("Using temperature: %v for model %s", s.config.DeepseekTemperature, modelName)

	// Add file contents if provided
	if len(filePaths) > 0 {
		// First, gather file contents to be included in the prompt
		fileContents := "\n\n# Reference Files\n"
		successfulFiles := 0
		fileSizes := []int64{}
		
		for _, filePath := range filePaths {
			// Read file content using our readFile function
			content, err := readFile(filePath)
			if err != nil {
				logger.Error("Failed to read file %s: %v", filePath, err)
				continue
			}
			
			// Record successful file read and size
			successfulFiles++
			fileSizes = append(fileSizes, int64(len(content)))
			
			// Get language extension for markdown highlighting
			language := getLanguageFromPath(filePath)
			
			// Add file content to the combined contents with file name as header and proper markdown formatting
			fileContents += fmt.Sprintf("\n\n## %s\n\n```%s\n%s\n```", 
				filepath.Base(filePath), language, string(content))
		}
		
		// Log some statistics about the files
		logger.Info("Including %d file(s) in the query, total size: %s", 
			successfulFiles, humanReadableSize(sumSizes(fileSizes)))
		
		// Create a chat request with file contents embedded in the query
		if successfulFiles > 0 {
			query = query + fileContents
		} else {
			logger.Warn("No files were successfully read to include in the query")
		}
	}
	
	// Update the request with the full query (either original or with file contents)
	request.Messages[1].Content = query
	
	// Send the request to the DeepSeek API
	response, err := s.client.CreateChatCompletion(ctx, request)
	if err != nil {
		logger.Error("DeepSeek API error: %v", err)
		errorMsg := fmt.Sprintf("Error from DeepSeek API: %v", err)
		
		// Include additional information in the error response
		if len(filePaths) > 0 {
			errorMsg += fmt.Sprintf("\n\nThe request included %d file(s).", len(filePaths))
		}
		
		return createErrorResponse(errorMsg), nil
	}
	
	return s.formatResponse(response), nil
}



// handleTokenEstimate handles requests to the deepseek_token_estimate tool
func (s *DeepseekServer) handleTokenEstimate(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Estimating token count")

	// Check if we have text or file_path
	text, hasText := req.Arguments["text"].(string)
	filePath, hasFilePath := req.Arguments["file_path"].(string)

	// Initialize variables for the response
	var estimatedTokens int
	var sourceType string
	var sourceName string
	var content string

	// Handle input from file path
	if hasFilePath && filePath != "" {
		// Read file content
		fileContent, err := readFile(filePath)
		if err != nil {
			logger.Error("Failed to read file: %v", err)
			return createErrorResponse(fmt.Sprintf("Error reading file: %v", err)), nil
		}

		// Convert to string for estimation
		content = string(fileContent)
		sourceType = "file"
		sourceName = filepath.Base(filePath)

		// Estimate tokens
		estimate := deepseek.EstimateTokenCount(content)
		estimatedTokens = estimate.EstimatedTokens

		logger.Info("Estimated %d tokens for file %s", estimatedTokens, filePath)
	} else if hasText && text != "" {
		// Estimate tokens directly from the provided text
		content = text
		sourceType = "text"
		sourceName = "provided input"

		// Estimate tokens
		estimate := deepseek.EstimateTokenCount(content)
		estimatedTokens = estimate.EstimatedTokens

		logger.Info("Estimated %d tokens for provided text", estimatedTokens)
	} else {
		// Neither text nor file_path provided
		return createErrorResponse("Please provide either 'text' or 'file_path' parameter"), nil
	}

	// Create a formatted response
	var formattedContent strings.Builder

	// Write the header
	formattedContent.WriteString("# Token Estimation Results\n\n")

	// Write the summary
	formattedContent.WriteString(fmt.Sprintf("**Source Type:** %s\n", sourceType))
	formattedContent.WriteString(fmt.Sprintf("**Source:** %s\n", sourceName))
	formattedContent.WriteString(fmt.Sprintf("**Estimated Token Count:** %d\n\n", estimatedTokens))

	// Add size information
	contentSize := len(content)
	charCount := len([]rune(content))

	formattedContent.WriteString("## Content Statistics\n\n")
	formattedContent.WriteString(fmt.Sprintf("- **Byte Size:** %s (%d bytes)\n", humanReadableSize(int64(contentSize)), contentSize))
	formattedContent.WriteString(fmt.Sprintf("- **Character Count:** %d characters\n", charCount))
	if charCount > 0 {
		formattedContent.WriteString(fmt.Sprintf("- **Tokens per Character Ratio:** %.2f tokens/char\n", float64(estimatedTokens)/float64(charCount)))
	}

	// Add usage note
	formattedContent.WriteString("\n## Note\n\n")
	formattedContent.WriteString("*This is an estimation and may not exactly match the token count used by the API. ")
	formattedContent.WriteString("Actual token usage can vary based on the model and specific tokenization algorithm.*\n")

	// Return the response
	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: formattedContent.String(),
			},
		},
	}, nil
}

// handleDeepseekBalance handles requests to the deepseek_balance tool
func (s *DeepseekServer) handleDeepseekBalance(ctx context.Context) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Checking DeepSeek API balance")

	// Get balance information from the API
	balanceResponse, err := deepseek.GetBalance(s.client, ctx)
	if err != nil {
		logger.Error("Failed to get balance from DeepSeek API: %v", err)
		return createErrorResponse(fmt.Sprintf("Error checking balance: %v", err)), nil
	}

	// Create a formatted response
	var formattedContent strings.Builder

	// Write the header
	formattedContent.WriteString("# DeepSeek API Balance Information\n\n")

	// Add availability status
	formattedContent.WriteString(fmt.Sprintf("**Account Status:** %s\n\n", 
		getAvailabilityStatus(balanceResponse.IsAvailable)))

	// If there are balance details, add them
	if len(balanceResponse.BalanceInfos) > 0 {
		formattedContent.WriteString("## Balance Details\n\n")
		formattedContent.WriteString("| Currency | Total Balance | Granted Balance | Topped-up Balance |\n")
		formattedContent.WriteString("|----------|--------------|----------------|------------------|\n")

		for _, balance := range balanceResponse.BalanceInfos {
			formattedContent.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				balance.Currency,
				balance.TotalBalance,
				balance.GrantedBalance,
				balance.ToppedUpBalance))
		}
	} else {
		formattedContent.WriteString("*No balance details available*\n")
	}

	// Add usage information
	formattedContent.WriteString("\n## Usage Information\n\n")
	formattedContent.WriteString("To top up your account or check more detailed usage statistics, ")
	formattedContent.WriteString("please visit the [DeepSeek Platform](https://platform.deepseek.com).\n")

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: formattedContent.String(),
			},
		},
	}, nil
}

// Helper function to format the availability status
func getAvailabilityStatus(isAvailable bool) string {
	if isAvailable {
		return "✅ Available (Balance is sufficient for API calls)"
	}
	return "❌ Unavailable (Insufficient balance for API calls)"
}

// handleDeepseekModels handles requests to the deepseek_models tool
func (s *DeepseekServer) handleDeepseekModels(ctx context.Context) (*protocol.CallToolResponse, error) {
	logger := getLoggerFromContext(ctx)
	logger.Info("Listing available DeepSeek models")

	// Get available models (dynamically discovered or fallback)
	models := s.GetAvailableDeepseekModels()
	
	// Try to refresh the models list if it's empty
	if len(models) == 0 {
		logger.Warn("No models available, attempting to refresh from API")
		err := s.discoverModels(ctx)
		if err != nil {
			logger.Error("Failed to refresh models from API: %v", err)
		} else {
			// Get the refreshed models
			models = s.GetAvailableDeepseekModels()
		}
	}

	// Create a formatted response using strings.Builder with error handling
	var formattedContent strings.Builder

	// Define a helper function to write with error checking
	writeStringf := func(format string, args ...interface{}) error {
		_, err := formattedContent.WriteString(fmt.Sprintf(format, args...))
		return err
	}

	// Write the header
	if err := writeStringf("# Available DeepSeek Models\n\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	// Write each model's information
	for _, model := range models {
		if err := writeStringf("## %s\n", model.Name); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		// Add basic model info
		if err := writeStringf("- ID: `%s`\n", model.ID); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}

		if err := writeStringf("- Description: %s\n\n", model.Description); err != nil {
			logger.Error("Error writing to response: %v", err)
			return createErrorResponse("Error generating model list"), nil
		}
	}

	// Add usage hint
	if err := writeStringf("## Usage\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("You can specify a model ID in the `model` parameter when using the `deepseek_ask` tool:\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	if err := writeStringf("```json\n{\n  \"query\": \"Your question here\",\n  \"model\": \"deepseek-chat\"\n}\n```\n"); err != nil {
		logger.Error("Error writing to response: %v", err)
		return createErrorResponse("Error generating model list"), nil
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: formattedContent.String(),
			},
		},
	}, nil
}

// executeDeepseekRequest makes the request to the DeepSeek API with retry capability
func (s *DeepseekServer) executeDeepseekRequest(ctx context.Context, model string, query string) (*deepseek.ChatCompletionResponse, error) {
	logger := getLoggerFromContext(ctx)

	var response *deepseek.ChatCompletionResponse

	// Define the operation to retry
	operation := func() error {
		var err error
		// Set timeout context for the API call
		timeoutCtx, cancel := context.WithTimeout(ctx, s.config.HTTPTimeout)
		defer cancel()

		request := &deepseek.ChatCompletionRequest{
			Model: model,
			Messages: []deepseek.ChatCompletionMessage{
				{
					Role:    deepseek.ChatMessageRoleUser,
					Content: query,
				},
			},
			Temperature: s.config.DeepseekTemperature,
		}
		response, err = s.client.CreateChatCompletion(timeoutCtx, request)
		if err != nil {
			logger.Error("DeepSeek API error: %v", err)
			return err
		}

		return nil
	}

	// Execute the operation with retry logic
	err := RetryWithBackoff(
		ctx,
		s.config.MaxRetries,
		s.config.InitialBackoff,
		s.config.MaxBackoff,
		operation,
		IsRetryableError, // Using the IsRetryableError from retry.go
		logger,
	)

	if err != nil {
		return nil, err
	}

	return response, nil
}

// formatResponse formats the DeepSeek API response
func (s *DeepseekServer) formatResponse(resp *deepseek.ChatCompletionResponse) *protocol.CallToolResponse {
	// Extract text from the response
	var content string
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	// Check for empty content and provide a fallback message
	if content == "" {
		content = "The DeepSeek model returned an empty response. This might indicate that the model couldn't generate an appropriate response for your query. Please try rephrasing your question or providing more context."
	}

	return &protocol.CallToolResponse{
		Content: []protocol.ToolContent{
			{
				Type: "text",
				Text: content,
			},
		},
	}
}

// Helper function to read a file
// This is declared at package level so it can be used by other files in the package
func readFile(path string) ([]byte, error) {
	// Use os.ReadFile to read the file from the file system
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return content, nil
}

// Helper function to get MIME type from file path
func getMimeTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	case ".wav":
		return "audio/wav"
	case ".doc", ".docx":
		return "application/msword"
	case ".xls", ".xlsx":
		return "application/vnd.ms-excel"
	case ".ppt", ".pptx":
		return "application/vnd.ms-powerpoint"
	case ".zip":
		return "application/zip"
	case ".csv":
		return "text/csv"
	case ".go":
		return "text/x-go"
	case ".py":
		return "text/x-python"
	case ".java":
		return "text/x-java"
	case ".c", ".cpp", ".h", ".hpp":
		return "text/x-c"
	case ".rb":
		return "text/plain"
	case ".php":
		return "text/plain"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

// getLanguageFromPath returns the language identifier for syntax highlighting based on file extension
func getLanguageFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".java":
		return "java"
	case ".json":
		return "json"
	case ".xml":
		return "xml"
	case ".md":
		return "markdown"
	case ".c":
		return "c"
	case ".cpp", ".hpp":
		return "cpp"
	case ".h":
		return "c"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".ts":
		return "typescript"
	case ".sh", ".bash":
		return "bash"
	case ".sql":
		return "sql"
	case ".yaml", ".yml":
		return "yaml"
	case ".rs":
		return "rust"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".groovy":
		return "groovy"
	case ".pl":
		return "perl"
	case ".r":
		return "r"
	case ".m":
		return "matlab"
	case ".ps1":
		return "powershell"
	case ".cs":
		return "csharp"
	case ".fs":
		return "fsharp"
	case ".vb":
		return "vbnet"
	case ".dart":
		return "dart"
	case ".ex", ".exs":
		return "elixir"
	case ".erl":
		return "erlang"
	case ".hs":
		return "haskell"
	case ".lua":
		return "lua"
	case ".jl":
		return "julia"
	case ".clj":
		return "clojure"
	// Default to text for unknown file types
	default:
		return "text"
	}
}

// sumSizes calculates the sum of an array of sizes
func sumSizes(sizes []int64) int64 {
	var total int64 = 0
	for _, size := range sizes {
		total += size
	}
	return total
}

// humanReadableSize formats a size in bytes to a human-readable string
func humanReadableSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}