package bootstrap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/voocel/agentcore"
)

func TestModelReasoningPrecedenceAndFallbackOnWire(t *testing.T) {
	const deepseek, minimax = "deepseek-v4-flash", "MiniMax-M2.5"
	type request struct {
		Model    string
		Thinking struct{ Type string }
	}
	calls := make(chan request, 16)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		calls <- body
		w.Header().Set("Content-Type", "application/json")
		if body.Model == minimax {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":{"message":"rate limit","type":"rate_limit_error"}}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	cfg := Config{Provider: "deepseek", ModelName: deepseek, ReasoningEffort: "high",
		Providers: map[string]ProviderConfig{"deepseek": {BaseURL: upstream.URL, APIKey: "stub"}, "minimax": {BaseURL: upstream.URL, APIKey: "stub"}},
		Roles:     map[string]RoleConfig{"writer": {Provider: "minimax", Model: minimax, ReasoningEffort: "off", Fallbacks: []ModelRef{{Provider: "deepseek", Model: deepseek}}}},
	}
	set, err := NewModelSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	generate := func(model agentcore.ChatModel, opts ...agentcore.CallOption) {
		t.Helper()
		if _, err := model.Generate(t.Context(), []agentcore.Message{agentcore.UserMsg("test")}, nil, opts...); err != nil {
			t.Fatal(err)
		}
	}
	generate(set.Default)
	generate(set.Default, agentcore.WithThinking(agentcore.ThinkingAuto))
	generate(set.ForRoleWithFailover("writer", nil))
	// Reconfiguring both the model and the default preserves in-flight facts
	// and updates a wrapper the caller already holds.
	held := set.Default
	started := held.requestTarget()
	cfg.ModelName, cfg.ReasoningEffort = "deepseek-v4-pro", "off"
	next, err := NewModelSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	set.ApplyPrepared(next)
	generate(started.model, started.callOptions(nil)...)
	generate(held)
	want := []struct{ model, mode string }{
		{deepseek, "enabled"}, {deepseek, ""},
		// MiniMax M2 cannot disable thinking. The same writer preference must
		// still reach the DeepSeek fallback, which can honor it.
		{minimax, ""}, {deepseek, "disabled"},
		{deepseek, "enabled"}, {cfg.ModelName, "disabled"},
	}
	if len(calls) != len(want) {
		t.Fatalf("got %d requests, want %d", len(calls), len(want))
	}
	for i, w := range want {
		got := <-calls
		if got.Model != w.model || got.Thinking.Type != w.mode {
			t.Errorf("request %d: %+v, want %s / %q", i+1, got, w.model, w.mode)
		}
	}
}
