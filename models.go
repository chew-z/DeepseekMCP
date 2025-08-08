package main

import (
	"errors"
	"fmt"
	"strings"
)

// DeepseekModelInfo holds information about a DeepSeek model
type DeepseekModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GetModelByIDFromServer returns a specific model by ID from the server's discovered models, or nil if not found
func (s *DeepseekServer) GetModelByID(modelID string) *DeepseekModelInfo {
	models := s.GetAvailableDeepseekModels()
	for _, model := range models {
		if model.ID == modelID {
			return &model
		}
	}
	return nil
}

// ValidateModelID checks if a model ID is in the list of available models from the server
// Returns nil if valid, error otherwise
func (s *DeepseekServer) ValidateModelID(modelID string) error {
	if s.GetModelByID(modelID) != nil {
		return nil
	}

	// Model not found, return error with available models
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Invalid model ID: %s. Available models are:", modelID))
	for _, model := range s.GetAvailableDeepseekModels() {
		sb.WriteString(fmt.Sprintf("\n- %s: %s", model.ID, model.Name))
	}

	return errors.New(sb.String())
}

// getFallbackDeepseekModels returns a hardcoded list of DeepSeek models as a fallback
func getFallbackDeepseekModels() []DeepseekModelInfo {
	return []DeepseekModelInfo{
		{
			ID:          "deepseek-chat",
			Name:        "DeepSeek Chat",
			Description: "General-purpose chat model from DeepSeek, balancing performance and efficiency",
		},
		{
			ID:          "deepseek-coder",
			Name:        "DeepSeek Coder",
			Description: "Specialized model for coding and technical tasks",
		},
		{
			ID:          "deepseek-reasoner",
			Name:        "DeepSeek Reasoner",
			Description: "Model optimized for reasoning and problem-solving tasks",
		},
	}
}

// GetAvailableDeepseekModels returns a list of available DeepSeek models from the server
// If no models were discovered from the API, it returns the fallback models
func (s *DeepseekServer) GetAvailableDeepseekModels() []DeepseekModelInfo {
	// Get models with thread safety
	s.modelsMu.RLock()
	models := s.models
	s.modelsMu.RUnlock()

	// If we have discovered models, return them
	if len(models) > 0 {
		return models
	}

	// Otherwise, return fallback hardcoded models
	return getFallbackDeepseekModels()
}
