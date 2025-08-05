package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cohesion-org/deepseek-go"
	mcp "github.com/mark3labs/mcp-go/mcp" // Changed import
)

// DeepseekServer implements the ToolHandler interface for DeepSeek API interactions
type DeepseekServer struct {
	config   *Config
	client   *deepseek.Client
	models   []DeepseekModelInfo // Dynamically discovered models
	modelsMu sync.RWMutex        // Mutex for thread-safe model access
	logger   Logger              // Added
}

// NewDeepseekServer creates a new DeepseekServer with the provided configuration
func NewDeepseekServer(ctx context.Context, config *Config) (*DeepseekServer, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	if config.DeepseekAPIKey == "" {
		return nil, errors.New("DeepSeek API key is required")
	}

	client := deepseek.NewClient(config.DeepseekAPIKey)
	// Unset DEEPSEEK_TIMEOUT to prevent the library from reading it
	os.Unsetenv("DEEPSEEK_TIMEOUT")

	logger := getLoggerFromContext(ctx) // Get logger instance

	server := &DeepseekServer{
		config: config,
		client: client,
		logger: logger, // Initialize logger
	}

	err := server.discoverModels(ctx)
	if err != nil {
		server.logger.Warn("Failed to discover DeepSeek models, will use fallback models: %v", err) // Use s.logger
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

	// Get models from the API with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, s.config.HTTPTimeout)
	defer cancel()
	apiModels, err := deepseek.ListAllModels(s.client, timeoutCtx)
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

// --- New Tool Handlers ---

// handleAskDeepseek handles requests to the ask_deepseek tool
func (s *DeepseekServer) handleAskDeepseek(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.logger.Info("Handling deepseek_ask request")

	query, err := req.RequireString("query")
	if err != nil {
		s.logger.Error("Missing required 'query' parameter: %v", err)
		return mcp.NewToolResultError("Missing required 'query' parameter: " + err.Error()), nil
	}

	modelName := s.config.DeepseekModel
	if customModel := req.GetString("model", ""); customModel != "" {
		if err := s.ValidateModelID(customModel); err != nil {
			s.logger.Error("Invalid model requested: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Invalid model specified: %v", err)), nil
		}
		s.logger.Info("Using request-specific model: %s", customModel)
		modelName = customModel
	}

	systemPrompt := s.config.DeepseekSystemPrompt
	if customPrompt := req.GetString("systemPrompt", ""); customPrompt != "" {
		s.logger.Info("Using request-specific system prompt")
		systemPrompt = customPrompt
	}

	filePaths := req.GetStringSlice("file_paths", nil) // Changed to GetStringSlice with a default

	jsonMode := req.GetBool("json_mode", false) // Added default value
	if jsonMode {
		s.logger.Info("JSON mode is enabled via request")
	}

	chatMessages := []deepseek.ChatCompletionMessage{
		{Role: deepseek.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: deepseek.ChatMessageRoleUser, Content: query},
	}

	finalQuery := query
	if len(filePaths) > 0 {
		s.logger.Info("Processing %d file_paths for context", len(filePaths))
		fileContents := "\n\n# Reference Files\n"
		successfulFiles := 0
		var fileSizes []int64

		// Define allowed directories for file access
		allowedDirs := s.config.AllowedFilePaths

		for _, filePath := range filePaths {
			// Security check: Ensure file path is within allowed directories
			if !isPathAllowed(filePath, allowedDirs) {
				s.logger.Error("Attempted to access file outside allowed directories: %s", filePath)
				continue
			}

			contentBytes, err := readFile(filePath)
			if err != nil {
				s.logger.Error("Failed to read file %s: %v", filePath, err)
				continue
			}
			successfulFiles++
			fileSizes = append(fileSizes, int64(len(contentBytes)))
			language := getLanguageFromPath(filePath)
			fileContents += fmt.Sprintf("\n\n## %s\n\n```%s\n%s\n```",
				filepath.Base(filePath), language, string(contentBytes))}

			if successfulFiles > 0 {
				s.logger.Info("Including %d file(s) in the query, total size: %s",
					successfulFiles, humanReadableSize(sumSizes(fileSizes)))
				finalQuery = query + fileContents
			} else {
				s.logger.Warn("No files were successfully read to include in the query")
			}
		}

		chatMessages[1].Content = finalQuery

		requestPayload := &deepseek.ChatCompletionRequest{
			Model:       modelName,
			Messages:    chatMessages,
			Temperature: s.config.DeepseekTemperature,
			JSONMode:    jsonMode,
		}

		s.logger.Debug("Using temperature: %v for model %s. JSON mode: %v", s.config.DeepseekTemperature, modelName, jsonMode)

		// Create timeout context for the API call
		timeoutCtx, cancel := context.WithTimeout(ctx, s.config.HTTPTimeout)
		defer cancel()
		
		response, err := s.client.CreateChatCompletion(timeoutCtx, requestPayload)
		if err != nil {
			s.logger.Error("DeepSeek API error: %v", err)
			errorMsg := fmt.Sprintf("Error from DeepSeek API: %v", err)
			if len(filePaths) > 0 {
				errorMsg += fmt.Sprintf("\n\nThe request included %d file(s).", len(filePaths))
			}
			return mcp.NewToolResultError(errorMsg), nil
		}

		var responseContent string
		if len(response.Choices) > 0 {
			responseContent = response.Choices[0].Message.Content
		}
		if responseContent == "" {
			s.logger.Warn("DeepSeek model returned an empty response.")
			responseContent = "The DeepSeek model returned an empty response. This might indicate that the model couldn't generate an appropriate response for your query. Please try rephrasing your question or providing more context."
		}
		return mcp.NewToolResultText(responseContent), nil
	}

	// handleDeepseekModels handles requests to the deepseek_models tool
	func (s *DeepseekServer) handleDeepseekModels(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		s.logger.Info("Listing available DeepSeek models")

		models := s.GetAvailableDeepseekModels()
		if len(models) == 0 {
			s.logger.Warn("No models available, attempting to refresh from API")
			err := s.discoverModels(ctx)
			if err != nil {
				s.logger.Error("Failed to refresh models from API: %v", err)
			} else {
				models = s.GetAvailableDeepseekModels()
			}
		}

		var formattedContent strings.Builder
		writeStringf := func(format string, args ...interface{}) {
			formattedContent.WriteString(fmt.Sprintf(format, args...))
		}

		writeStringf("# Available DeepSeek Models\n\n")
		for _, model := range models {
			writeStringf("## %s\n", model.Name)
			writeStringf("- ID: `%s`\n", model.ID)
			writeStringf("- Description: %s\n\n", model.Description)
		}
		writeStringf("## Usage\n")
		writeStringf("You can specify a model ID in the `model` parameter when using the `deepseek_ask` tool:\n")
		writeStringf("```json\n{\n  \"query\": \"Your question here\",\n  \"model\": \"deepseek-chat\"\n}\n```\n")

		return mcp.NewToolResultText(formattedContent.String()), nil
	}

	// handleDeepseekBalance handles requests to the deepseek_balance tool
	func (s *DeepseekServer) handleDeepseekBalance(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		s.logger.Info("Checking DeepSeek API balance")

		timeoutCtx, cancel := context.WithTimeout(ctx, s.config.HTTPTimeout)
	defer cancel()
	balanceResponse, err := deepseek.GetBalance(s.client, timeoutCtx)
		if err != nil {
			s.logger.Error("Failed to get balance from DeepSeek API: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Error checking balance: %v", err)), nil
		}

		var formattedContent strings.Builder
		formattedContent.WriteString("# DeepSeek API Balance Information\n\n")
		formattedContent.WriteString(fmt.Sprintf("**Account Status:** %s\n\n", getAvailabilityStatus(balanceResponse.IsAvailable)))

		if len(balanceResponse.BalanceInfos) > 0 {
			formattedContent.WriteString("## Balance Details\n\n")
			formattedContent.WriteString("| Currency | Total Balance | Granted Balance | Topped-up Balance |\n")
			formattedContent.WriteString("|----------|--------------|----------------|------------------|\n")
			for _, balance := range balanceResponse.BalanceInfos {
				formattedContent.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
					balance.Currency, balance.TotalBalance, balance.GrantedBalance, balance.ToppedUpBalance))
			}
		} else {
			formattedContent.WriteString("*No balance details available*\n")
		}
		formattedContent.WriteString("\n## Usage Information\n\n")
		formattedContent.WriteString("To top up your account or check more detailed usage statistics, ")
		formattedContent.WriteString("please visit the [DeepSeek Platform](https://platform.deepseek.com).\n")

		return mcp.NewToolResultText(formattedContent.String()), nil
	}

	// handleTokenEstimate handles requests to the deepseek_token_estimate tool
	func (s *DeepseekServer) handleTokenEstimate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		s.logger.Info("Estimating token count")

		text := req.GetString("text", "")
		filePath := req.GetString("file_path", "")

		var estimatedTokens int
		var sourceType string
		var sourceName string
		var contentToEstimate string

		if filePath != "" {
			// Security check: Ensure file path is within allowed directories
			if !isPathAllowed(filePath, s.config.AllowedFilePaths) {
				s.logger.Error("Attempted to access file outside allowed directories: %s", filePath)
				return mcp.NewToolResultError(fmt.Sprintf("Access to file path is denied: %s", filePath)), nil
			}

			fileContentBytes, err := readFile(filePath)
			if err != nil {
				s.logger.Error("Failed to read file for token estimation %s: %v", filePath, err)
				return mcp.NewToolResultError(fmt.Sprintf("Error reading file: %v", err)), nil
			}
			contentToEstimate = string(fileContentBytes)
			sourceType = "file"
			sourceName = filepath.Base(filePath)
			estimate := deepseek.EstimateTokenCount(contentToEstimate)
			estimatedTokens = estimate.EstimatedTokens
			s.logger.Info("Estimated %d tokens for file %s", estimatedTokens, filePath)
		} else if text != "" {
			contentToEstimate = text
			sourceType = "text"
			sourceName = "provided input"
			estimate := deepseek.EstimateTokenCount(contentToEstimate)
			estimatedTokens = estimate.EstimatedTokens
			s.logger.Info("Estimated %d tokens for provided text", estimatedTokens)
		} else {
			s.logger.Warn("handleTokenEstimate called without 'text' or 'file_path'")
			return mcp.NewToolResultError("Please provide either 'text' or 'file_path' parameter"), nil
		}

	var formattedResponse strings.Builder
	formattedResponse.WriteString("# Token Estimation Results\n\n")
	formattedResponse.WriteString(fmt.Sprintf("**Source Type:** %s\n", sourceType))
	formattedResponse.WriteString(fmt.Sprintf("**Source:** %s\n", sourceName))
	formattedResponse.WriteString(fmt.Sprintf("**Estimated Token Count:** %d\n\n", estimatedTokens))
	contentSize := len(contentToEstimate)
	charCount := len([]rune(contentToEstimate))
	formattedResponse.WriteString("## Content Statistics\n\n")
	formattedResponse.WriteString(fmt.Sprintf("- **Byte Size:** %s (%d bytes)\n", humanReadableSize(int64(contentSize)), contentSize))
	formattedResponse.WriteString(fmt.Sprintf("- **Character Count:** %d characters\n", charCount))
	if charCount > 0 {
		formattedResponse.WriteString(fmt.Sprintf("- **Tokens per Character Ratio:** %.2f tokens/char\n", float64(estimatedTokens)/float64(charCount)))
	}
	formattedResponse.WriteString("\n## Note\n\n")
	formattedResponse.WriteString("*This is an estimation and may not exactly match the token count used by the API. ")
	formattedResponse.WriteString("Actual token usage can vary based on the model and specific tokenization algorithm.*\n")

	return mcp.NewToolResultText(formattedResponse.String()), nil
}

// getLoggerFromContext safely extracts a logger from the context or creates a new one
// This assumes loggerKey and Logger type are defined (e.g., in main.go or a shared types.go)
// and NewLogger, LevelInfo are available.
func getLoggerFromContext(ctx context.Context) Logger {
	loggerValue := ctx.Value(loggerKey)
	if loggerValue != nil {
		if l, ok := loggerValue.(Logger); ok {
			return l
		}
	}
	return NewLogger(LevelInfo)
}

// Helper function to format the availability status
func getAvailabilityStatus(isAvailable bool) string {
	if isAvailable {
		return "✅ Available (Balance is sufficient for API calls)"
	}
	return "❌ Unavailable (Insufficient balance for API calls)"
}


// formatResponse was here, now removed.

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
