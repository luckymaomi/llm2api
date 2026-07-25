package httpserver

import (
	"net/http"
	"strings"
)

type routeRegistrar interface {
	Get(string, http.HandlerFunc)
}

func documentationRoutes(router routeRegistrar) {
	router.Get("/llms.txt", serveAgentDocumentation)
	router.Get("/openapi.json", serveOpenAPI)
}

func serveAgentDocumentation(w http.ResponseWriter, r *http.Request) {
	baseURL := requestOrigin(r) + "/v1"
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("# LLM2API\n\n" +
		"LLM2API is an OpenAI-compatible API gateway.\n\n" +
		"## Connection\n\n" +
		"- Base URL: " + baseURL + "\n" +
		"- Authentication: Authorization: Bearer $LLM2API_API_KEY\n" +
		"- Discover the models available to this key with GET " + baseURL + "/models before sending a request.\n\n" +
		"## Endpoints\n\n" +
		"- GET " + baseURL + "/models returns the models available to the current API key.\n" +
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
		"Machine-readable OpenAPI: " + requestOrigin(r) + "/openapi.json\n"))
}

func serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	baseURL := requestOrigin(r) + "/v1"
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, map[string]any{
		"openapi": "3.1.0",
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
		},
		"paths": map[string]any{
			"/models": map[string]any{
				"get": map[string]any{
					"summary":   "List models available to the current API key",
					"security":  []map[string][]string{{"bearerAuth": {}}},
					"responses": map[string]any{"200": map[string]string{"description": "OpenAI-compatible model list"}},
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

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
