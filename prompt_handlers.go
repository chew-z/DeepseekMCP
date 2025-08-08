package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// createTaskInstructions generates the instructional text for the MCP client
func createTaskInstructions(problemStatement, systemPrompt string) string {
	return fmt.Sprintf("You MUST NOW use the `deepseek_ask` tool to solve this problem.\n\n" +
		"Follow these instructions carefully:\n" +
		"1. Set the `query` argument to a clear and concise request based on the user's problem statement.\n" +
		"2. Provide the code to be analyzed using ONE of the following methods:\n" +
		"   - Use the `file_paths` argument for one or more files.\n" +
		"   - Embed a code snippet directly into the `query` argument.\n" +
		"3. Use the following text for the `systemPrompt` argument:\n\n" +
		"<system_prompt>\n%s\n</system_prompt>\n\n" +
		"<problem_statement>\n%s\n</problem_statement>", systemPrompt, problemStatement)
}

// promptHandler is the generic handler for all prompts
func (s *DeepseekServer) promptHandler(p *PromptDefinition) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		problemStatement, ok := req.Params.Arguments["problem_statement"]
		if !ok || problemStatement == "" {
			return nil, fmt.Errorf("problem_statement argument is required")
		}

		instructions := createTaskInstructions(problemStatement, p.SystemPrompt.GetSystemPrompt())

		return mcp.NewGetPromptResult(
			req.Params.Name,
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.RoleAssistant, mcp.NewTextContent(instructions)),
			},
		), nil
	}
}
