package providers

import (
	"fmt"
	"net/url"
	"regexp"
	"time"
)

var presetSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

type ProviderPreset struct {
	ID         string
	Slug       string
	Name       string
	Kind       Kind
	BaseURL    string
	SourceURL  string
	VerifiedAt string
}

func (c *Catalog) Presets() []ProviderPreset {
	if c == nil {
		return nil
	}
	return cloneProviderPresets(c.presets)
}

func (c *Catalog) Preset(id string) (ProviderPreset, bool) {
	if c == nil {
		return ProviderPreset{}, false
	}
	preset, found := c.presetByID[id]
	return cloneProviderPreset(preset), found
}

func validateProviderPreset(preset ProviderPreset) error {
	if preset.ID == "" || !presetSlugPattern.MatchString(preset.Slug) || preset.Name == "" || preset.Kind == "" {
		return fmt.Errorf("id, slug, name, and kind are required")
	}
	for label, value := range map[string]string{"base URL": preset.BaseURL, "source URL": preset.SourceURL} {
		parsed, err := url.ParseRequestURI(value)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return fmt.Errorf("%s must be an absolute HTTPS URL", label)
		}
	}
	if _, err := time.Parse(time.DateOnly, preset.VerifiedAt); err != nil {
		return fmt.Errorf("verified date must use YYYY-MM-DD")
	}
	return nil
}

func cloneProviderPresets(presets []ProviderPreset) []ProviderPreset {
	result := make([]ProviderPreset, len(presets))
	for index, preset := range presets {
		result[index] = cloneProviderPreset(preset)
	}
	return result
}

func cloneProviderPreset(preset ProviderPreset) ProviderPreset {
	return preset
}
