package publicapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/luckymaomi/llm2api/internal/canonical"
	"github.com/luckymaomi/llm2api/internal/httpserver"
	"github.com/luckymaomi/llm2api/internal/protocol"
)

func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := httpserver.RequestIDFromContext(r.Context())
		scheme, secret, found := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(secret) == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="LLM2API"`)
			protocol.WriteError(w, requestID, authenticationError())
			return
		}
		principal, err := a.identity.AuthenticateGatewayKey(r.Context(), strings.TrimSpace(secret))
		secret = ""
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="LLM2API"`)
			protocol.WriteError(w, requestID, authenticationError())
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal)))
	})
}

func authenticationError() *canonical.Error {
	return &canonical.Error{Kind: canonical.ErrorAuthentication, Code: "invalid_api_key", Message: "invalid API key", HTTPStatus: http.StatusUnauthorized}
}
