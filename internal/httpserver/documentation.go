package httpserver

import (
	"net/http"

	"github.com/luckymaomi/llm2api/internal/protocol"
)

type routeRegistrar interface {
	Get(string, http.HandlerFunc)
}

func documentationRoutes(router routeRegistrar, publicOrigin string) {
	router.Get("/llms.txt", serveAgentDocumentation(publicOrigin))
	router.Get("/openapi.json", serveOpenAPI(publicOrigin))
}

func serveAgentDocumentation(publicOrigin string) http.HandlerFunc {
	baseURL := publicOrigin + "/v1"
	openAPIURL := publicOrigin + "/openapi.json"
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("# LLM2API\n\n" +
			"LLM2API is the only Provider exposed by this OpenAI-compatible API gateway. Internal upstream Providers, resource pools, and credentials are never part of the client contract.\n\n" +
			"## Connection\n\n" +
			"- Provider: " + protocol.PublicProvider + "\n" +
			"- Base URL: " + baseURL + "\n" +
			"- Authentication: Authorization: Bearer $LLM2API_API_KEY\n" +
			"- Discover the models available to this key with GET " + baseURL + "/models before sending a request.\n\n" +
			"## Endpoints\n\n" +
			"- GET " + baseURL + "/models returns the models available to the current API key, including each model's machine-readable capabilities and limits.\n" +
			"- POST " + baseURL + "/chat/completions accepts the OpenAI Chat Completions request format.\n" +
			"- Set stream: true for Server-Sent Events. Stop on data: [DONE].\n\n" +
			"## Responses\n\n" +
			"- POST " + baseURL + "/responses accepts the OpenAI Responses request format.\n" +
			"- Use store: true to retain a response. A stored response can be read with GET " + baseURL + "/responses/{response_id}.\n" +
			"- A background response uses background: true and store: true. Cancel it with POST " + baseURL + "/responses/{response_id}/cancel.\n\n" +
			"## Retry behavior\n\n" +
			"- Treat 400 and 401 as configuration errors.\n" +
			"- On 429, honor Retry-After when present.\n" +
			"- Only retry a request when no response bytes were received and the operation is safe to retry.\n\n" +
			"Machine-readable OpenAPI: " + openAPIURL + "\n"))
	}
}

func serveOpenAPI(publicOrigin string) http.HandlerFunc {
	baseURL := publicOrigin + "/v1"
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		WriteJSON(w, http.StatusOK, map[string]any{
			"openapi":    "3.1.0",
			"x-provider": protocol.PublicProvider,
			"info": map[string]string{
				"title":       "LLM2API",
				"version":     "1.0.0",
				"description": "OpenAI-compatible gateway. Discover models available to the current API key before making requests.",
			},
			"servers": []map[string]string{{"url": baseURL}},
			"components": map[string]any{
				"securitySchemes": map[string]any{
					"bearerAuth": map[string]string{"type": "http", "scheme": "bearer"},
				},
				"schemas": map[string]any{
					"IntegerParameterLimit": map[string]any{
						"type": "object", "required": []string{"supported"},
						"properties": map[string]any{"supported": map[string]string{"type": "boolean"}, "minimum": map[string]string{"type": "integer", "format": "int64"}, "maximum": map[string]string{"type": "integer", "format": "int64"}, "exact_values": map[string]any{"type": "array", "items": map[string]string{"type": "integer", "format": "int64"}}},
					},
					"NumberParameterLimit": map[string]any{
						"type": "object", "required": []string{"supported"},
						"properties": map[string]any{"supported": map[string]string{"type": "boolean"}, "minimum": map[string]string{"type": "number"}, "maximum": map[string]string{"type": "number"}, "exact_values": map[string]any{"type": "array", "items": map[string]string{"type": "number"}}},
					},
					"SamplingCondition": map[string]any{
						"type": "object", "properties": map[string]any{"thinking_enabled": map[string]string{"type": "boolean"}, "temperature_exact": map[string]string{"type": "number"}, "temperature_at_most": map[string]string{"type": "number"}, "n_maximum": map[string]string{"type": "integer", "format": "int64"}},
					},
					"ModelParameters": map[string]any{
						"type": "object", "required": []string{"max_completion_tokens", "temperature", "top_p", "presence_penalty", "frequency_penalty", "n", "top_k", "thinking_budget"},
						"properties": map[string]any{
							"max_completion_tokens": map[string]string{"$ref": "#/components/schemas/IntegerParameterLimit"}, "temperature": map[string]string{"$ref": "#/components/schemas/NumberParameterLimit"}, "top_p": map[string]string{"$ref": "#/components/schemas/NumberParameterLimit"}, "presence_penalty": map[string]string{"$ref": "#/components/schemas/NumberParameterLimit"}, "frequency_penalty": map[string]string{"$ref": "#/components/schemas/NumberParameterLimit"}, "n": map[string]string{"$ref": "#/components/schemas/IntegerParameterLimit"}, "top_k": map[string]string{"$ref": "#/components/schemas/NumberParameterLimit"}, "thinking_budget": map[string]string{"$ref": "#/components/schemas/IntegerParameterLimit"}, "sampling_conditions": map[string]any{"type": "array", "items": map[string]string{"$ref": "#/components/schemas/SamplingCondition"}},
						},
					},
					"ModelCapabilities": map[string]any{
						"type": "object", "required": []string{"chat_completions", "responses", "streaming", "tools", "reasoning", "vision", "structured_output", "extensions", "usage", "limits", "parameters"},
						"properties": map[string]any{
							"chat_completions": map[string]string{"type": "boolean"}, "responses": map[string]string{"type": "boolean"}, "streaming": map[string]string{"type": "boolean"},
							"tools": map[string]any{"type": "object", "properties": map[string]any{
								"function_calling": map[string]string{"type": "boolean"}, "tool_choice": map[string]any{"type": "array", "items": map[string]string{"type": "string"}},
								"strict_schema": map[string]string{"type": "boolean"}, "parallel_tool_calls": map[string]string{"type": "boolean"}, "streaming_tool_calls": map[string]string{"type": "boolean"},
							}},
							"reasoning":         map[string]any{"type": "object", "properties": map[string]any{"enabled": map[string]string{"type": "boolean"}, "mode": map[string]string{"type": "string"}, "always_on": map[string]string{"type": "boolean"}, "default_enabled": map[string]string{"type": "boolean"}, "configurable": map[string]string{"type": "boolean"}, "content": map[string]string{"type": "boolean"}, "preserve": map[string]string{"type": "boolean"}, "efforts": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}, "tool_choice_modes_when_enabled": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}}},
							"vision":            map[string]any{"type": "object", "properties": map[string]any{"image_url": map[string]string{"type": "boolean"}, "video_url": map[string]string{"type": "boolean"}}},
							"structured_output": map[string]any{"type": "object", "properties": map[string]any{"json_object": map[string]string{"type": "boolean"}, "json_schema": map[string]string{"type": "boolean"}}},
							"extensions":        map[string]any{"type": "object", "properties": map[string]any{"partial_mode": map[string]string{"type": "boolean"}, "message_name": map[string]string{"type": "boolean"}, "prompt_cache_key": map[string]string{"type": "boolean"}, "safety_identifier": map[string]string{"type": "boolean"}}},
							"usage":             map[string]any{"type": "object", "properties": map[string]any{"response": map[string]string{"type": "boolean"}, "stream": map[string]string{"type": "boolean"}}},
							"limits":            map[string]any{"type": "object", "properties": map[string]any{"context_tokens": map[string]string{"type": "integer", "format": "int64"}, "output_tokens": map[string]string{"type": "integer", "format": "int64"}}},
							"parameters":        map[string]string{"$ref": "#/components/schemas/ModelParameters"},
						},
					},
				},
			},
			"paths": map[string]any{
				"/models": map[string]any{
					"get": map[string]any{
						"summary":  "List models available to the current API key",
						"security": []map[string][]string{{"bearerAuth": {}}},
						"responses": map[string]any{
							"200": map[string]any{
								"description": "Model list with capability contracts",
								"content": map[string]any{
									"application/json": map[string]any{
										"schema": map[string]any{
											"type": "object",
											"properties": map[string]any{
												"data": map[string]any{
													"type": "array",
													"items": map[string]any{
														"type": "object",
														"properties": map[string]any{
															"id":           map[string]string{"type": "string"},
															"owned_by":     map[string]any{"type": "string", "const": protocol.PublicProvider},
															"capabilities": map[string]string{"$ref": "#/components/schemas/ModelCapabilities"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				"/chat/completions": map[string]any{
					"post": map[string]any{
						"summary":   "Create an OpenAI-compatible chat completion",
						"security":  []map[string][]string{{"bearerAuth": {}}},
						"responses": map[string]any{"200": map[string]string{"description": "Completion or Server-Sent Events when stream is true"}},
					},
				},
				"/responses": map[string]any{
					"post": map[string]any{
						"summary":   "Create an OpenAI-compatible Response",
						"security":  []map[string][]string{{"bearerAuth": {}}},
						"responses": map[string]any{"200": map[string]string{"description": "Response or Server-Sent Events when stream is true"}},
					},
				},
				"/responses/{responseID}": map[string]any{
					"get": map[string]any{
						"summary":   "Read a stored Response",
						"security":  []map[string][]string{{"bearerAuth": {}}},
						"responses": map[string]any{"200": map[string]string{"description": "Stored Response"}},
					},
					"delete": map[string]any{
						"summary":   "Delete a stored Response",
						"security":  []map[string][]string{{"bearerAuth": {}}},
						"responses": map[string]any{"200": map[string]string{"description": "Deletion result"}},
					},
				},
				"/responses/{responseID}/input_items": map[string]any{
					"get": map[string]any{
						"summary":   "List stored input items",
						"security":  []map[string][]string{{"bearerAuth": {}}},
						"responses": map[string]any{"200": map[string]string{"description": "Input item page"}},
					},
				},
				"/responses/{responseID}/cancel": map[string]any{
					"post": map[string]any{
						"summary":   "Cancel a stored background Response",
						"security":  []map[string][]string{{"bearerAuth": {}}},
						"responses": map[string]any{"200": map[string]string{"description": "Canceled Response"}},
					},
				},
			},
		})
	}
}
