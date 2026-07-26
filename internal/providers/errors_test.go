package providers

import (
	"net/http"
	"testing"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

func TestProviderErrorFixturesProduceStableKinds(t *testing.T) {
	t.Parallel()

	silicon, err := NewSiliconFlow(SiliconFlowOptions{
		BaseURL: "https://llm.example/v1", Capabilities: SiliconFlowCapabilities(),
	})
	if err != nil {
		t.Fatalf("create SiliconFlow adapter: %v", err)
	}
	tests := []struct {
		name       string
		adapter    Adapter
		statusCode int
		body       string
		wantKind   canonical.ErrorKind
		wantCode   string
		wantReplay bool
	}{
		{
			name: "Zhipu platform overload", adapter: NewZhipu(), statusCode: http.StatusTooManyRequests,
			body:     `{"error":{"code":"1305","message":"Model traffic is high"}}`,
			wantKind: canonical.ErrorProviderTemporary, wantCode: "1305",
		},
		{
			name: "Zhipu account rate limit", adapter: NewZhipu(), statusCode: http.StatusTooManyRequests,
			body:     `{"error":{"code":1302,"message":"Rate limit reached"}}`,
			wantKind: canonical.ErrorRateLimit, wantCode: "1302", wantReplay: true,
		},
		{
			name: "Agnes authentication", adapter: NewAgnes(), statusCode: http.StatusUnauthorized,
			body:     `{"error":{"message":"Authentication failed","type":"authentication_error","code":"invalid_api_key"}}`,
			wantKind: canonical.ErrorAuthentication, wantCode: "invalid_api_key", wantReplay: true,
		},
		{
			name: "SiliconFlow invalid parameters", adapter: silicon, statusCode: http.StatusUnprocessableEntity,
			body:     `{"error":{"message":"Invalid parameter","type":"invalid_request_error","param":"max_tokens","code":"invalid_value"}}`,
			wantKind: canonical.ErrorInvalidRequest, wantCode: "invalid_value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			headers := http.Header{"Retry-After": []string{"17"}}
			classified := test.adapter.ClassifyError(test.statusCode, headers, []byte(test.body))
			if classified.Kind != test.wantKind || classified.Code != test.wantCode {
				t.Fatalf("classified error = %#v", classified)
			}
			if classified.ReplaySafe != test.wantReplay {
				t.Fatalf("replay-safe = %t, want %t", classified.ReplaySafe, test.wantReplay)
			}
			if classified.RetryAfter == nil || classified.RetryAfter.DelaySeconds == nil || *classified.RetryAfter.DelaySeconds != 17 {
				t.Fatalf("retry-after = %#v", classified.RetryAfter)
			}
		})
	}
}
