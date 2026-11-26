package host

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hansshaw/OpenNovel/assets"
	"github.com/hansshaw/OpenNovel/internal/bootstrap"
)

func TestDeepSeekStartAndResumeArbitrationHonorReasoning(t *testing.T) {
	const prompt = "规划一部关于海港信使的短篇"
	for _, tc := range []struct {
		name         string
		recoverFirst bool
	}{
		{"start", false}, {"resume after changing settings", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var arbitrations atomic.Int32
			workerStarted := make(chan struct{}, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Stream      bool
					Thinking    struct{ Type string }
					Temperature *float64
					Messages    []struct{ Content string }
					Format      *struct{ Type string } `json:"response_format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body.Thinking.Type != "disabled" || body.Temperature == nil || *body.Temperature != 0 {
					t.Errorf("lost reasoning or sampling on real model request: %+v", body)
				}
				if body.Stream {
					select {
					case workerStarted <- struct{}{}:
					default:
					}
					<-r.Context().Done()
					return
				}
				content := "{}"
				if len(body.Messages) > 0 && strings.Contains(body.Messages[0].Content, "PLAN_START_FIXTURE") {
					arbitrations.Add(1)
					if body.Format != nil {
						t.Errorf("unexpected native response format with json_schema disabled: %+v", body.Format)
					}
					content = `{"planner":"architect_short","task":"写出海港的短篇规划","reason":"篇幅紧凑"}`
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, content)
			}))
			t.Cleanup(upstream.Close)
			const model = "deepseek-v4-flash"
			disabled := false
			cfg := bootstrap.Config{Provider: "deepseek", ModelName: model, OutputDir: t.TempDir(),
				ReasoningEffort: "off",
				Providers: map[string]bootstrap.ProviderConfig{"deepseek": {BaseURL: upstream.URL, APIKey: "stub",
					Models:    []bootstrap.ModelConfig{{Name: model, ContextWindow: 65536, JSONSchema: &disabled}},
					ExtraBody: map[string]any{"temperature": 0}}}}
			cfg.Notify.Enabled = &disabled
			if tc.recoverFirst {
				cfg.ReasoningEffort = ""
			}
			cfg.FillDefaults()
			bundle := assets.Load(cfg.Style, assets.DefaultLoadOptions(cfg.OutputDir))
			bundle.Prompts.ArbiterPlanStart = "PLAN_START_FIXTURE"
			h, err := New(cfg, bundle, WithUserDataDir(t.TempDir()))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(h.Close)
			err = h.StartPrepared(prompt)
			if tc.recoverFirst {
				if err == nil || !strings.Contains(err.Error(), "thinking mode") || arbitrations.Load() != 0 {
					t.Fatalf("unset reasoning must keep provider validation before HTTP: %v", err)
				}
				if err := h.SetRoleThinking("default", "off"); err != nil {
					t.Fatal(err)
				}
				if _, err := h.Resume(); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			select {
			case <-workerStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("planner was never dispatched")
			}
			meta, err := h.store.RunMeta.Load()
			if err != nil || meta == nil || meta.PlanStart == nil || meta.PlanStart.RawPrompt != prompt || meta.PlanStart.Planner != "architect_short" || arbitrations.Load() != 1 {
				t.Fatalf("startup fact not committed exactly once: %+v, err=%v, calls=%d", meta, err, arbitrations.Load())
			}
		})
	}
}
