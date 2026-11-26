package bootstrap

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hansshaw/OpenNovel/internal/errs"
	"github.com/voocel/litellm"
)

// Standard sampling fields belong to the typed litellm request; provider options
// are a separate namespace and are validated by each adapter.
func (pc ProviderConfig) requestParameters() (litellm.RequestDefaults, map[string]any, error) {
	var defaults litellm.RequestDefaults
	extra := cloneMap(pc.ExtraBody)
	for key, value := range extra {
		switch strings.ToLower(key) {
		case "model", "messages", "input", "instructions", "system", "tools", "tool_choice",
			"response_format", "stream", "max_tokens", "max_completion_tokens", "max_output_tokens",
			"api_key", "authorization", "headers", "base_url":
			return defaults, nil, fmt.Errorf("extra_body.%s is managed by the runtime or provider configuration: %w", key, errs.ErrConfig)
		}
		switch key {
		case "temperature", "top_p":
			limit := 2.0
			if key == "top_p" {
				limit = 1
			}
			// JSON round-trip accepts numeric Go types as well as decoded JSON,
			// and rejects null, numeric strings, NaN and infinity.
			raw, err := json.Marshal(value)
			var number *float64
			if err != nil || json.Unmarshal(raw, &number) != nil || number == nil || *number < 0 || *number > limit {
				return defaults, nil, fmt.Errorf("extra_body.%s must be a number between 0 and %g: %w", key, limit, errs.ErrConfig)
			}
			if key == "temperature" {
				defaults.Temperature = number
			} else {
				defaults.TopP = number
			}
			delete(extra, key)
		}
	}
	return defaults, extra, nil
}
