package main

import (
	"context"

	"github.com/cohesion-org/deepseek-go"
)

// DeepseekAPI defines the interface for the DeepSeek client.
// This allows for mocking the client in tests.
type DeepseekAPI interface {
	CreateChatCompletion(ctx context.Context, req *deepseek.ChatCompletionRequest) (*deepseek.ChatCompletionResponse, error)
	ListAllModels(ctx context.Context) (*deepseek.APIModels, error)
	GetBalance(ctx context.Context) (*deepseek.BalanceResponse, error)
}

// realDeepseekClient is an adapter for the real deepseek.Client to satisfy the DeepseekAPI interface.
type realDeepseekClient struct {
	client *deepseek.Client
}

func (r *realDeepseekClient) CreateChatCompletion(ctx context.Context, req *deepseek.ChatCompletionRequest) (*deepseek.ChatCompletionResponse, error) {
	return r.client.CreateChatCompletion(ctx, req)
}

func (r *realDeepseekClient) ListAllModels(ctx context.Context) (*deepseek.APIModels, error) {
	return deepseek.ListAllModels(r.client, ctx)
}

func (r *realDeepseekClient) GetBalance(ctx context.Context) (*deepseek.BalanceResponse, error) {
	return deepseek.GetBalance(r.client, ctx)
}
