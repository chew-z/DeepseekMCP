# DeepSeek MCP Server

A production-grade MCP server integrating with DeepSeek's API, featuring advanced code review capabilities, efficient file management, and API account management.

## Features

- **Multi-Model Support**: Choose from various DeepSeek models including DeepSeek Chat and DeepSeek Coder
- **Code Review Focus**: Built-in system prompt for detailed code analysis with markdown output
- **Automatic File Handling**: Built-in file management with direct path integration
- **API Account Management**: Check balance and estimate token usage
- **JSON Mode Support**: Request structured JSON responses for easy parsing
- **Advanced Error Handling**: Graceful degradation with structured error logging
- **Improved Retry Logic**: Automatic retries with configurable exponential backoff for API calls
- **Security**: Configurable file type restrictions and size limits
- **Performance Monitoring**: Built-in metrics collection for request latency and throughput

## Prerequisites

- Go 1.21+
- DeepSeek API key
- Basic understanding of MCP protocol

## Installation & Quick Start

```bash
# Clone and build
git clone https://github.com/your-username/DeepseekMCP
cd DeepseekMCP
go build -o bin/mcp-deepseek

# Start server with environment variables
export DEEPSEEK_API_KEY=your_api_key
export DEEPSEEK_MODEL=deepseek-chat
./bin/mcp-deepseek
```

## Configuration

### Claude Desktop

```json
{
  "mcpServers": {
    "deepseek": {
      "command": "/Your/project/path/bin/mcp-deepseek",
      "env": {
        "DEEPSEEK_API_KEY": "YOUR_API_KEY",
        "DEEPSEEK_MODEL": "deepseek-chat"
      }
    }
  }
}
```


### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DEEPSEEK_API_KEY` | DeepSeek API key | *Required* |
| `DEEPSEEK_MODEL` | Model ID from available models | `deepseek-chat` |
| `DEEPSEEK_SYSTEM_PROMPT` | System prompt for code review | *Default code review prompt* |
| `DEEPSEEK_SYSTEM_PROMPT_FILE` | Path to file containing system prompt | Empty |
| `DEEPSEEK_MAX_FILE_SIZE` | Max upload size (bytes) | `10485760` (10MB) |
| `DEEPSEEK_ALLOWED_FILE_TYPES` | Comma-separated MIME types | [Common text/code types] |
| `DEEPSEEK_TIMEOUT` | API timeout in seconds | `90` |
| `DEEPSEEK_MAX_RETRIES` | Max API retries | `2` |
| `DEEPSEEK_INITIAL_BACKOFF` | Initial backoff time (seconds) | `1` |
| `DEEPSEEK_MAX_BACKOFF` | Maximum backoff time (seconds) | `10` |
| `DEEPSEEK_TEMPERATURE` | Model temperature (0.0-1.0) | `0.4` |

Example `.env`:
```env
DEEPSEEK_API_KEY=your_api_key
DEEPSEEK_MODEL=deepseek-chat
DEEPSEEK_SYSTEM_PROMPT="Your custom code review prompt here"
# Alternative: load system prompt from file
# DEEPSEEK_SYSTEM_PROMPT_FILE=/path/to/prompt.txt
DEEPSEEK_MAX_FILE_SIZE=5242880  # 5MB
DEEPSEEK_ALLOWED_FILE_TYPES=text/x-go,text/markdown
DEEPSEEK_TEMPERATURE=0.7
```

## Core API Tools

Currently, the server provides the following tools:

### deepseek_ask

Used for code analysis, review, and general queries with optional file path inclusion.

```json
{
  "name": "deepseek_ask",
  "arguments": {
    "query": "Review this Go code for concurrency issues...",
    "model": "deepseek-chat",
    "systemPrompt": "Optional custom review instructions",
    "file_paths": ["main.go", "config.go"],
    "json_mode": false
  }
}
```

### deepseek_models

Lists all available DeepSeek models with their capabilities.

```json
{
  "name": "deepseek_models",
  "arguments": {}
}
```

### deepseek_balance

Checks your DeepSeek API account balance and availability status.

```json
{
  "name": "deepseek_balance",
  "arguments": {}
}
```

### deepseek_token_estimate

Estimates the token count for text or a file to help with quota management.

```json
{
  "name": "deepseek_token_estimate",
  "arguments": {
    "text": "Your text to estimate...",
    "file_path": "path/to/your/file.go"
  }
}
```

## Supported Models

The following DeepSeek models are supported by default:

| Model ID | Description |
|----------|-------------|
| `deepseek-chat` | General-purpose chat model balancing performance and efficiency |
| `deepseek-coder` | Specialized model for coding and technical tasks |
| `deepseek-reasoner` | Model optimized for reasoning and problem-solving tasks |

*Note: The actual available models may vary based on your API access level. The server will automatically discover and make available all models you have access to through the DeepSeek API.*

## Supported File Types
| Extension | MIME Type |
|-----------|-----------|
| .go       | text/x-go |
| .py       | text/x-python |
| .js       | text/javascript |
| .md       | text/markdown |
| .java     | text/x-java |
| .c/.h     | text/x-c |
| .cpp/.hpp | text/x-c++ |
| 25+ more  | (See `getMimeTypeFromPath` in deepseek.go) |

## Operational Notes

- **Degraded Mode**: Automatically enters safe mode on initialization errors
- **Audit Logging**: All operations logged with timestamps and metadata
- **Security**: File content validated by MIME type and size before processing

## File Handling

The server handles files directly through the `deepseek_ask` tool:

1. Specify local file paths in the `file_paths` array parameter
2. The server automatically:
   - Reads the files from the provided paths
   - Determines the correct MIME type based on file extension
   - Uploads the file content to the DeepSeek API
   - Uses the files as context for the query

This direct file handling approach eliminates the need for separate file upload/management endpoints.

## JSON Mode Support

For integrations that require structured data output, the server supports JSON mode:

- **Structured Responses**: Request properly formatted JSON responses from DeepSeek models
- **Parser-Friendly**: Ideal for CI/CD pipelines and automation systems
- **Easy Integration**: Simply set `json_mode: true` in your request

Example with JSON mode:
```json
{
  "name": "deepseek_ask",
  "arguments": {
    "query": "Analyze this code and return a JSON object with: issues_found (array of strings), complexity_score (number 1-10), and recommendations (array of strings)",
    "model": "deepseek-chat",
    "json_mode": true,
    "file_paths": ["main.go", "config.go"]
  }
}
```

This returns a well-formed JSON response that can be parsed directly by your application.

## Development

### Command-line Options

The server supports these command-line options to override environment variables:

```bash
# Override the DeepSeek model to use
./bin/mcp-deepseek -deepseek-model=deepseek-coder

# Override the system prompt
./bin/mcp-deepseek -deepseek-system-prompt="Your custom prompt here"

# Override the temperature setting (0.0-1.0)
./bin/mcp-deepseek -deepseek-temperature=0.8
```

### Running Tests

To run tests:

```bash
go test -v ./...
```

### Running Linter

```bash
golangci-lint run
```

### Formatting Code

```bash
gofmt -w .
```

## License

[MIT License](LICENSE)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the project
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request
