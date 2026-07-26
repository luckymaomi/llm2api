package publicapi

import (
	"net/http"

	"github.com/luckymaomi/llm2api/internal/httpserver"
)

func (a *API) models(w http.ResponseWriter, r *http.Request) {
	principal := principalFromContext(r.Context())
	models, err := a.catalog.Models(r.Context(), principal.KeyID)
	if err != nil {
		a.logger.Error("list public models failed", "request_id", httpserver.RequestIDFromContext(r.Context()), "error", err)
		httpserver.WriteProblem(w, httpserver.Problem{Type: "about:blank", Title: "Service unavailable", Status: http.StatusServiceUnavailable, Code: "model_catalog_unavailable", RequestID: httpserver.RequestIDFromContext(r.Context())})
		return
	}
	data := make([]map[string]any, 0, len(models))
	for _, model := range models {
		data = append(data, map[string]any{
			"id": model.PublicName, "object": "model", "created": model.CreatedAt.Unix(), "owned_by": model.ProviderSlug,
			"capabilities": map[string]any{
				"chat_completions": model.Capabilities.Chat,
				"streaming":        model.Capabilities.Streaming,
				"tools": map[string]any{
					"function_calling":     model.Capabilities.Tools,
					"tool_choice":          model.Capabilities.ToolChoiceModes,
					"strict_schema":        model.Capabilities.StrictTools,
					"parallel_tool_calls":  model.Capabilities.ParallelToolCalls,
					"streaming_tool_calls": model.Capabilities.ToolStreaming,
				},
				"reasoning": map[string]any{
					"enabled":                        model.Capabilities.Reasoning,
					"mode":                           model.Capabilities.ReasoningMode,
					"always_on":                      model.Capabilities.ReasoningAlwaysOn,
					"default_enabled":                model.Capabilities.ReasoningDefaultEnabled,
					"preserve":                       model.Capabilities.ReasoningPreserve,
					"efforts":                        model.Capabilities.ReasoningEfforts,
					"tool_choice_modes_when_enabled": model.Capabilities.ToolChoiceModesWithReasoning,
				},
				"vision": map[string]any{"image_url": model.Capabilities.ImageInput, "video_url": model.Capabilities.VideoInput},
				"structured_output": map[string]any{
					"json_object": model.Capabilities.StructuredOutput,
					"json_schema": model.Capabilities.JSONSchemaOutput,
				},
				"extensions": map[string]any{
					"partial_mode":      model.Capabilities.PartialMode,
					"prompt_cache_key":  model.Capabilities.PromptCacheKey,
					"safety_identifier": model.Capabilities.SafetyIdentifier,
				},
				"limits": map[string]int64{
					"context_tokens": model.Capabilities.ContextTokens,
					"output_tokens":  model.Capabilities.OutputTokens,
				},
				"parameters": model.Capabilities.Parameters,
			},
		})
	}
	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}
