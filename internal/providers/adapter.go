package providers

import (
	"context"
	"net/http"
	"time"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

type Kind string

const (
	KindSiliconFlow Kind = "siliconflow"
	KindZhipu       Kind = "zhipu"
	KindAgnes       Kind = "agnes"
	KindKimi        Kind = "kimi"
)

type Credential struct {
	APIKey string
}

type ProbeKind string

const (
	ProbeModels ProbeKind = "models"
)

type Probe struct {
	Available        bool
	MayConsumeTokens bool
	Kind             ProbeKind
	Request          *http.Request
}

type StatusProbeKind string

const (
	StatusProbeKimiBalance StatusProbeKind = "kimi_balance"
)

type StatusProbe struct {
	Available bool
	Kind      StatusProbeKind
	Request   *http.Request
}

type UpstreamStatusState string

const (
	UpstreamStatusObserved    UpstreamStatusState = "observed"
	UpstreamStatusUnknown     UpstreamStatusState = "unknown"
	UpstreamStatusUnavailable UpstreamStatusState = "unavailable"
)

type UpstreamStatusScope string

const (
	UpstreamStatusScopeAccount    UpstreamStatusScope = "account"
	UpstreamStatusScopeProject    UpstreamStatusScope = "project"
	UpstreamStatusScopeCredential UpstreamStatusScope = "credential"
	UpstreamStatusScopeUnknown    UpstreamStatusScope = "unknown"
)

// UpstreamStatusObservation contains only facts supplied by a documented
// Provider endpoint or by a documented response header. It intentionally does
// not turn a gateway-local rate bucket into an upstream quota claim.
type UpstreamStatusObservation struct {
	State      UpstreamStatusState `json:"state"`
	Scope      UpstreamStatusScope `json:"scope"`
	ObservedAt time.Time           `json:"observed_at"`
	Source     string              `json:"source"`
	Reason     string              `json:"reason,omitempty"`
	Balance    *UpstreamBalance    `json:"balance,omitempty"`
}

// UpstreamBalance uses decimal strings so financial facts never pass through
// binary floating point in the gateway contract.
type UpstreamBalance struct {
	Currency  string `json:"currency"`
	Available string `json:"available"`
	Voucher   string `json:"voucher"`
	Cash      string `json:"cash"`
}

type DiscoveredModel struct {
	ID string
}

type StreamParser interface {
	Feed([]byte) ([]canonical.StreamEvent, error)
	Close() ([]canonical.StreamEvent, error)
}

type Adapter interface {
	Kind() Kind
	Capabilities() Capabilities
	BuildRequest(context.Context, Credential, canonical.ChatRequest) (*http.Request, error)
	ParseResponse(statusCode int, headers http.Header, body []byte) (canonical.ChatResponse, error)
	ParseStream() StreamParser
	ClassifyError(statusCode int, headers http.Header, body []byte) *canonical.Error
	Probe(context.Context, Credential) (Probe, error)
	ParseProbe(ProbeKind, int, http.Header, []byte) ([]DiscoveredModel, *canonical.Error)
	ValidateProbe(ProbeKind, int, http.Header, []byte) *canonical.Error
	StatusProbe(context.Context, Credential) (StatusProbe, error)
	ParseStatusProbe(StatusProbeKind, int, http.Header, []byte) (UpstreamStatusObservation, *canonical.Error)
}
