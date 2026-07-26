package requestflow

import (
	"net/http"
	"sync"

	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/security"
)

type ProviderFactory struct {
	policy     security.SSRFPolicy
	catalog    *providers.Catalog
	clientOnce sync.Once
	client     *http.Client
	clientErr  error
}

func NewProviderFactory(policy security.SSRFPolicy) *ProviderFactory {
	return &ProviderFactory{policy: policy, catalog: providers.DefaultCatalog()}
}

func (f *ProviderFactory) Adapter(model Model) (providers.Adapter, error) {
	return f.catalog.Build(model.ProviderKind, providers.AdapterOptions{BaseURL: model.ProviderBaseURL, Capabilities: model.Capabilities.AdapterCapabilities()})
}

func (f *ProviderFactory) Client(candidate Candidate) (*http.Client, error) {
	f.clientOnce.Do(func() {
		f.client, f.clientErr = security.NewSSRFSafeClient(f.policy)
		if f.client != nil {
			f.client.Timeout = 0
		}
	})
	return f.client, f.clientErr
}
