package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hansshaw/OpenNovel/internal/bootstrap"
	"github.com/hansshaw/OpenNovel/internal/store"
	"github.com/voocel/agentcore"
)

type coCreateEventsModel struct {
	events        []agentcore.StreamEvent
	streamCalls   int
	generateCalls int
}

func (m *coCreateEventsModel) Generate(context.Context, []agentcore.Message, []agentcore.ToolSpec, ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	m.generateCalls++
	return nil, fmt.Errorf("unexpected direct generation")
}

func (m *coCreateEventsModel) GenerateStream(context.Context, []agentcore.Message, []agentcore.ToolSpec, ...agentcore.CallOption) (<-chan agentcore.StreamEvent, error) {
	m.streamCalls++
	ch := make(chan agentcore.StreamEvent, len(m.events))
	for _, ev := range m.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func (*coCreateEventsModel) SupportsTools() bool { return false }

func coCreateTestModels(m *coCreateEventsModel) *bootstrap.ModelSet {
	return &bootstrap.ModelSet{Default: bootstrap.NewSwappableModel("test", "cocreate", m, nil)}
}

func coCreateTestResponse(reply, draft string, ready bool) string {
	return fmt.Sprintf("<reply>%s</reply><draft>%s</draft><ready>%t</ready><suggestions></suggestions>", reply, draft, ready)
}

func TestCoCreateStreamUsesAuthoritativeDone(t *testing.T) {
	stale := coCreateTestResponse("旧回复", "旧草稿", true)
	final := coCreateTestResponse("新回复", "新草稿", false)
	tests := []struct {
		name          string
		deltas        []agentcore.StreamEvent
		finalText     string
		finalThinking string
	}{
		{
			name: "recovered response replaces stale preview",
			deltas: []agentcore.StreamEvent{
				{Type: agentcore.StreamEventTextDelta, Delta: stale},
				{Type: agentcore.StreamEventThinkingDelta, Delta: "旧思考"},
			},
			finalText: final, finalThinking: "重取后的思考",
		},
		{
			name:      "done only response",
			finalText: final, finalThinking: "非流式思考",
		},
		{
			name:      "matching streamed text is not duplicated",
			deltas:    []agentcore.StreamEvent{{Type: agentcore.StreamEventTextDelta, Delta: final}},
			finalText: final,
		},
		{
			name:      "empty final thinking clears stale thinking",
			deltas:    []agentcore.StreamEvent{{Type: agentcore.StreamEventThinkingDelta, Delta: "旧思考"}},
			finalText: final,
		},
		{
			name:          "final thinking answer replaces stale text",
			deltas:        []agentcore.StreamEvent{{Type: agentcore.StreamEventTextDelta, Delta: stale}},
			finalThinking: final,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := store.NewStore(t.TempDir())
			if err := st.Init(); err != nil {
				t.Fatal(err)
			}
			model := &coCreateEventsModel{events: append(tt.deltas, agentcore.StreamEvent{
				Type: agentcore.StreamEventDone,
				Message: agentcore.Message{Role: agentcore.RoleAssistant, StopReason: agentcore.StopReasonStop, Content: []agentcore.ContentBlock{
					agentcore.TextBlock(tt.finalText), agentcore.ThinkingBlock(tt.finalThinking),
				}},
			})}
			progress := make(map[string]string)
			got, err := coCreateStream(context.Background(), coCreateTestModels(model), st.Sessions, coCreateSystemPrompt,
				[]CoCreateMessage{{Role: "user", Content: "继续共创"}}, func(kind, text string) { progress[kind] = text })
			if err != nil {
				t.Fatal(err)
			}
			if got.Message != "新回复" || got.Prompt != "新草稿" || got.Ready || got.Raw != final {
				t.Fatalf("must use only the complete final response: %+v", got)
			}
			if progress[CoCreateProgressReply] != "新回复" {
				t.Fatalf("stale reply preview survived final response: %q", progress[CoCreateProgressReply])
			}
			if progress[CoCreateProgressThinking] != tt.finalThinking {
				t.Fatalf("stale thinking survived final response: %q", progress[CoCreateProgressThinking])
			}
			data, err := os.ReadFile(filepath.Join(st.Dir(), "meta", "sessions", "cocreate.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			var logged coCreateLogEntry
			if err := json.Unmarshal(data, &logged); err != nil {
				t.Fatal(err)
			}
			if logged.RawResponse != tt.finalText || logged.Thinking != tt.finalThinking || logged.ParsedReady {
				t.Fatalf("session must record authoritative response only: %+v", logged)
			}
		})
	}
}

func TestCoCreateStreamRejectsIncompleteResponse(t *testing.T) {
	raw := coCreateTestResponse("可以开始", "不能采用的草稿", true)
	tests := []struct {
		name    string
		events  []agentcore.StreamEvent
		wantErr string
		partial bool
	}{
		{
			name:    "missing done with apparently ready content",
			events:  []agentcore.StreamEvent{{Type: agentcore.StreamEventTextDelta, Delta: raw}},
			partial: true,
		},
		{
			name: "length in final message",
			events: []agentcore.StreamEvent{{Type: agentcore.StreamEventDone, Message: agentcore.Message{
				Content: []agentcore.ContentBlock{agentcore.TextBlock(raw)}, StopReason: agentcore.StopReasonLength,
			}}},
			wantErr: "stop_reason=length",
		},
		{
			name: "length on done event",
			events: []agentcore.StreamEvent{{Type: agentcore.StreamEventDone, StopReason: agentcore.StopReasonLength,
				Message: agentcore.Message{Content: []agentcore.ContentBlock{agentcore.TextBlock(raw)}}}},
			wantErr: "stop_reason=length",
		},
		{
			name:    "empty authoritative final must not reuse stale ready response",
			events:  []agentcore.StreamEvent{{Type: agentcore.StreamEventTextDelta, Delta: raw}, {Type: agentcore.StreamEventDone}},
			wantErr: "empty response",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &coCreateEventsModel{events: tt.events}
			got, err := coCreateStream(context.Background(), coCreateTestModels(model), nil, coCreateSystemPrompt,
				[]CoCreateMessage{{Role: "user", Content: "继续共创"}}, nil)
			if err == nil || tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("incomplete response accepted or wrong error: reply=%+v, err=%v", got, err)
			}
			if tt.partial && !errors.Is(err, agentcore.ErrStreamPartial) {
				t.Fatalf("missing Done must preserve stream-partial identity: %v", err)
			}
			if got.Ready || got.Prompt != "" || got.Message != "" {
				t.Fatalf("incomplete response must not update draft or readiness: %+v", got)
			}
		})
	}
}

func TestCoCreateStreamCancellationDoesNotPublishDraftOrRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	raw := coCreateTestResponse("可以开始", "不能采用的草稿", true)
	model := &coCreateEventsModel{events: []agentcore.StreamEvent{
		{Type: agentcore.StreamEventTextDelta, Delta: raw},
		{Type: agentcore.StreamEventDone, Message: testTextMsg(raw)},
	}}
	got, err := coCreateStream(ctx, coCreateTestModels(model), nil, coCreateSystemPrompt,
		[]CoCreateMessage{{Role: "user", Content: "继续共创"}}, func(_, _ string) { cancel() })
	if !errors.Is(err, context.Canceled) || got.Ready || got.Prompt != "" {
		t.Fatalf("cancellation must preserve the previous draft: reply=%+v, err=%v", got, err)
	}
	if model.streamCalls != 1 || model.generateCalls != 0 {
		t.Fatalf("cancellation must not retry: stream=%d, generate=%d", model.streamCalls, model.generateCalls)
	}
}

func TestDeepSeekProbeAndCoCreationUseTheirOwnReasoningSettings(t *testing.T) {
	calls := make(chan string, 32)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model       string
			Stream      bool
			Thinking    struct{ Type string }
			Temperature *float64
			Messages    []struct{ Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body.Thinking.Type != "disabled" || body.Temperature == nil || *body.Temperature != 0 {
			t.Errorf("unexpected wire reasoning/sampling for %s: %+v, %v", body.Model, body.Thinking, body.Temperature)
		}
		if body.Stream {
			calls <- body.Model + ":stream"
			w.Header().Set("Content-Type", "text/event-stream")
			response, _ := json.Marshal(coCreateTestResponse("可以开始", "一部海港小说", true))
			fmt.Fprintf(w, "data: {\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"content\":%s},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", body.Model, response)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		result := "OK"
		if len(body.Messages) > 0 && body.Messages[0].Content == coCreateCompactPrompt {
			calls <- body.Model + ":summary"
			result = "<summary>保留海港和信件。</summary>"
		} else {
			calls <- body.Model + ":probe"
		}
		fmt.Fprintf(w, `{"model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, body.Model, result)
	}))
	defer upstream.Close()
	const general, selected = "deepseek-v4-flash", "deepseek-v4-pro"
	models := []bootstrap.ModelConfig{{Name: general, ContextWindow: 5000, MaxOutputTokens: 512}, {Name: selected, ContextWindow: 5000, MaxOutputTokens: 512}}
	cfg := bootstrap.Config{Provider: "local", ModelName: general, ReasoningEffort: "off",
		Providers: map[string]bootstrap.ProviderConfig{"local": {Type: "deepseek", BaseURL: upstream.URL, APIKey: "stub", Models: models, ExtraBody: map[string]any{"temperature": 0}}},
		Roles:     map[string]bootstrap.RoleConfig{"thinking": {Provider: "local", Model: selected, ReasoningEffort: "off"}}}
	set, err := bootstrap.NewModelSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := &Host{cfg: cfg, models: set, store: store.NewStore(t.TempDir())}
	draft := ModelConfigurationDraft{Provider: "local", Type: "deepseek", BaseURL: upstream.URL, Models: models}
	if err := h.TestModelConnection(t.Context(), draft, general); err != nil {
		t.Fatal(err)
	}
	if got := <-calls; got != general+":probe" {
		t.Fatalf("default probe called %s", got)
	}

	// A role-specific off must neither change the connection probe nor be
	// lost when that role's long conversation has to be summarized.
	h.cfg.ReasoningEffort = ""
	if err := h.TestModelConnection(t.Context(), draft, general); err == nil || !strings.Contains(err.Error(), "thinking mode") || len(calls) != 0 {
		t.Fatalf("probe should retain the provider's default restriction, got %v", err)
	}
	history := longCoCreateHistory(20)
	for i := range history {
		if history[i].Role == "assistant" {
			history[i].Content = coCreateTestResponse("是否继续", "保留海港和信件。", false)
		}
	}
	reply, err := h.CoCreateStream(t.Context(), history, nil)
	if err != nil || !reply.Ready || reply.Checkpoint == nil {
		t.Fatalf("co-creation: %+v, %v", reply, err)
	}
	if len(calls) < 2 {
		t.Fatal("expected both compaction and streaming calls")
	}
	for len(calls) > 1 {
		if got := <-calls; got != selected+":summary" {
			t.Fatalf("compaction called %s", got)
		}
	}
	if got := <-calls; got != selected+":stream" {
		t.Fatalf("co-creation called %s", got)
	}
}
