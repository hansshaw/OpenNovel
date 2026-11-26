package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hansshaw/OpenNovel/internal/errs"
	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/llm"
)

func TestProviderSamplingOnWire(t *testing.T) {
	const chatReply = `{"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
	for _, protocol := range append(llm.RegisteredProviders(), "responses", "stream") {
		t.Run(protocol, func(t *testing.T) {
			pc := ProviderConfig{Type: protocol, APIKey: "local-test", ExtraBody: map[string]any{"temperature": 0, "top_p": 0.95}}
			options := []agentcore.CallOption{agentcore.WithMaxTokens(937)}
			wantTemperature := 0.0
			container, topKey, maxKey, reply := "", "top_p", "max_tokens", chatReply
			switch protocol {
			case "openai", "responses", "stream":
				pc.Type = "openai"
				pc.ExtraBody["metadata"] = map[string]any{"stage": "writer"}
				if protocol == "responses" {
					pc.API = "responses"
					maxKey = "max_output_tokens"
					reply = `{"model":"test-model","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`
				}
			case "deepseek":
				options = append(options, agentcore.WithThinking(agentcore.ThinkingOff))
			case "anthropic":
				wantTemperature = 1
				pc.ExtraBody["temperature"] = 1
				delete(pc.ExtraBody, "top_p") // Anthropic accepts only one of the two sampling controls.
				reply = `{"id":"msg","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`
			case "gemini":
				container, topKey, maxKey = "generationConfig", "topP", "maxOutputTokens"
				reply = `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`
			case "bedrock":
				container, topKey, maxKey = "inferenceConfig", "topP", "maxTokens"
				pc.Extra = map[string]any{"region": "us-east-1", "access_key_id": "local", "secret_access_key": "local"}
				reply = `{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn"}`
			case "openrouter":
				pc.ExtraBody["min_p"] = 0.05
			}
			got := make(chan map[string]any, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				got <- body
				if protocol == "stream" {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, reply)
			}))
			defer upstream.Close()
			pc.BaseURL = upstream.URL
			model, err := createModelFromConfig("sample", "test-model", pc, 1100, make(map[string]agentcore.ChatModel))
			if err != nil {
				t.Fatal(err)
			}
			messages := []agentcore.Message{agentcore.UserMsg("write")}
			if protocol == "stream" {
				stream, err := model.GenerateStream(t.Context(), messages, nil, options...)
				if err != nil {
					t.Fatal(err)
				}
				for event := range stream {
					if event.Err != nil {
						t.Fatal(event.Err)
					}
				}
			} else if _, err := model.Generate(t.Context(), messages, nil, options...); err != nil {
				t.Fatal(err)
			}
			body := <-got
			params := body
			if container != "" {
				params, _ = body[container].(map[string]any)
			}
			if params["temperature"] != wantTemperature {
				t.Errorf("wrong temperature in %+v", params)
			}
			if protocol != "anthropic" && params[topKey] != 0.95 {
				t.Errorf("wrong top_p in %+v", params)
			}
			if params[maxKey] != 937.0 && params["max_completion_tokens"] != 937.0 {
				t.Errorf("per-call token budget missing in %+v", params)
			}
			if pc.Type == "openai" {
				metadata, _ := body["metadata"].(map[string]any)
				if metadata["stage"] != "writer" {
					t.Errorf("missing metadata: %+v", body)
				}
			}
			if protocol == "openrouter" && body["min_p"] != 0.05 {
				t.Errorf("missing provider extension: %+v", body)
			}
			if pc.ExtraBody["temperature"] != int(wantTemperature) {
				t.Fatal("saved provider config mutated")
			}
		})
	}
}

func TestProviderSamplingRejectsInvalidValuesAndRuntimeOverrides(t *testing.T) {
	for key, value := range map[string]any{"temperature": "0.8", "top_p": 1.2, "model": "other", "messages": "other", "max_output_tokens": 2000, "stream": true, "headers": map[string]any{}, "api_key": "private-test-marker"} {
		pc := ProviderConfig{Type: "openrouter", ExtraBody: map[string]any{key: value}}
		if _, err := createModelFromConfig("test", "m", pc, 40, map[string]agentcore.ChatModel{}); !errors.Is(err, errs.ErrConfig) || !strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "private-test-marker") {
			t.Errorf("invalid %s did not produce a configuration error naming only the field: %v", key, err)
		}
	}
}

func TestOpenAIStillValidatesVendorSpecificSamplingOptions(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unsupported option reached upstream") }))
	defer upstream.Close()
	pc := ProviderConfig{Type: "openai", APIKey: "local-test", BaseURL: upstream.URL, ExtraBody: map[string]any{"temperature": 0.8, "min_p": 0.05}}
	model, err := createModelFromConfig("custom", "test-model", pc, 200, map[string]agentcore.ChatModel{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = model.Generate(t.Context(), []agentcore.Message{agentcore.UserMsg("write")}, nil)
	if err == nil || !strings.Contains(err.Error(), "min_p") {
		t.Fatalf("expected native adapter to reject min_p: %v", err)
	}
}
