package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luckymaomi/llm2api/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

type readinessStub struct {
	err error
}

func (s readinessStub) Ready(context.Context) error { return s.err }

func TestHealthEndpointsExposeRuntimeState(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{HTTP: config.HTTP{MaxBodyBytes: 1024, PublicOrigin: "https://gateway.example"}}
	router := NewRouter(cfg, logger, readinessStub{}, prometheus.NewRegistry(), nil, nil)

	for _, path := range []string{"/health/live", "/health/ready"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, response.Code)
		}
		if response.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s did not expose a request ID", path)
		}
	}
}

func TestReadinessReportsDependencyFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{HTTP: config.HTTP{MaxBodyBytes: 1024, PublicOrigin: "https://gateway.example"}}
	router := NewRouter(cfg, logger, readinessStub{err: errors.New("database offline")}, prometheus.NewRegistry(), nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready returned %d", response.Code)
	}
}

func TestRequestIDRejectsUnsafeInput(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{HTTP: config.HTTP{MaxBodyBytes: 1024}}
	router := NewRouter(cfg, logger, readinessStub{}, prometheus.NewRegistry(), nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	request.Header.Set("X-Request-ID", "unsafe\nvalue")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Header().Get("X-Request-ID") == "unsafe\nvalue" {
		t.Fatal("unsafe request ID was reflected")
	}
}

func TestDocumentationEndpointsExposeStableMachineReadableEntryPoints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{HTTP: config.HTTP{MaxBodyBytes: 1024, PublicOrigin: "https://gateway.example"}}
	router := NewRouter(cfg, logger, readinessStub{}, prometheus.NewRegistry(), nil, nil)

	request := httptest.NewRequest(http.MethodGet, "http://internal.example/llms.txt", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("llms.txt returned %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("llms.txt content type = %q", got)
	}
	if body := response.Body.String(); !strings.Contains(body, "https://gateway.example/v1/models") || !strings.Contains(body, "Provider: llm2api") {
		t.Fatalf("llms.txt did not contain the public models endpoint: %s", body)
	}

	request = httptest.NewRequest(http.MethodGet, "http://internal.example/openapi.json", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("openapi.json returned %d", response.Code)
	}
	var document map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatalf("openapi.json is invalid JSON: %v", err)
	}
	if document["openapi"] != "3.1.0" {
		t.Fatalf("openapi version = %v", document["openapi"])
	}
	if document["x-provider"] != "llm2api" {
		t.Fatalf("openapi public Provider = %v", document["x-provider"])
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok || paths["/models"] == nil || paths["/chat/completions"] == nil || paths["/responses"] == nil {
		t.Fatalf("openapi paths are incomplete: %v", document["paths"])
	}
	components, ok := document["components"].(map[string]any)
	if !ok {
		t.Fatalf("openapi components are missing: %v", document)
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok || schemas["ModelParameters"] == nil || schemas["SamplingCondition"] == nil {
		t.Fatalf("openapi parameter capability schemas are missing: %v", components)
	}
}
