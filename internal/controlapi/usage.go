package controlapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/luckymaomi/llmgateway/internal/httpserver"
	"github.com/luckymaomi/llmgateway/internal/identity"
	"github.com/luckymaomi/llmgateway/internal/usage"
)

type usageService interface {
	ListRequestLogs(context.Context, identity.Principal, usage.RequestLogQuery) (usage.PageResult[usage.RequestLog], error)
	GetRequestLog(context.Context, identity.Principal, uuid.UUID) (usage.RequestLogDetail, error)
}

type UsageAPI struct {
	service usageService
	logger  *slog.Logger
	now     func() time.Time
}

func NewUsageAPI(service usageService, logger *slog.Logger) *UsageAPI {
	return &UsageAPI{service: service, logger: logger, now: time.Now}
}

func (a *UsageAPI) RegisterRoutes(router chi.Router, _, _ func(http.Handler) http.Handler) {
	router.Get("/requests", a.listRequestLogs)
	router.Get("/requests/{requestID}", a.getRequestLog)
}

func (a *UsageAPI) listRequestLogs(w http.ResponseWriter, r *http.Request) {
	query := parseListQuery(r)
	userID, userErr := optionalUUID(query.UserID)
	keyID, keyErr := optionalUUID(query.GatewayKeyID)
	modelID, modelErr := optionalUUID(query.ModelID)
	poolID, poolErr := optionalUUID(query.ResourcePoolID)
	from, to, windowOK := a.requestLogWindow(query.From, query.To)
	if userErr != nil || keyErr != nil || modelErr != nil || poolErr != nil || !windowOK {
		a.writeError(w, r, usage.ErrInvalidInput)
		return
	}
	result, err := a.service.ListRequestLogs(r.Context(), principalFromContext(r.Context()), usage.RequestLogQuery{
		UserID: userID, GatewayKeyID: keyID, ModelID: modelID, ResourcePoolID: poolID, Search: query.Search,
		Status: usage.RequestStatus(query.Status), From: from, To: to, Page: usage.Page{Offset: query.offset(), Size: int32(query.PageSize)},
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	views, err := presentRequestLogs(principalFromContext(r.Context()), result.Items)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, pageView[requestLogView]{Items: views, Page: query.Page, PageSize: query.PageSize, Total: result.Total})
}

func (a *UsageAPI) getRequestLog(w http.ResponseWriter, r *http.Request) {
	requestID, err := uuid.Parse(chi.URLParam(r, "requestID"))
	if err != nil {
		a.writeError(w, r, usage.ErrInvalidInput)
		return
	}
	detail, err := a.service.GetRequestLog(r.Context(), principalFromContext(r.Context()), requestID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	view, err := presentRequestLogDetail(principalFromContext(r.Context()), detail)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, view)
}

func (a *UsageAPI) requestLogWindow(fromValue, toValue string) (time.Time, time.Time, bool) {
	to := a.now().UTC()
	from := to.Add(-24 * time.Hour)
	var err error
	if strings.TrimSpace(toValue) != "" {
		to, err = time.Parse(time.RFC3339, toValue)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
	}
	if strings.TrimSpace(fromValue) != "" {
		from, err = time.Parse(time.RFC3339, fromValue)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
	} else if strings.TrimSpace(toValue) != "" {
		from = to.Add(-24 * time.Hour)
	}
	return from.UTC(), to.UTC(), to.After(from) && to.Sub(from) <= 31*24*time.Hour
}

func optionalUUID(value string) (*uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (a *UsageAPI) writeError(w http.ResponseWriter, r *http.Request, err error) {
	value := problem{Status: http.StatusInternalServerError, Code: "internal_error", Message: "Usage operation failed.", Retryable: true, Stage: "usage"}
	switch {
	case errors.Is(err, usage.ErrInvalidInput):
		value.Status, value.Code, value.Message, value.Retryable = http.StatusBadRequest, "invalid_request", "Usage query is invalid.", false
	case errors.Is(err, usage.ErrForbidden):
		value.Status, value.Code, value.Message, value.Retryable = http.StatusForbidden, "forbidden", "The current session cannot read these usage facts.", false
	case errors.Is(err, usage.ErrNotFound):
		value.Status, value.Code, value.Message, value.Retryable = http.StatusNotFound, "not_found", "Usage record was not found.", false
	default:
		if a.logger != nil {
			a.logger.Error("usage operation failed", "request_id", httpserver.RequestIDFromContext(r.Context()), "error", err)
		}
	}
	writeProblem(w, r, value)
}
