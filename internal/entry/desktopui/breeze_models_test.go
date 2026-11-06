package desktopui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func breezeConnectionDraft(base string) map[string]any {
	return map[string]any{
		"name": "本地 Breeze 配音", "kind": "audio", "protocol": "breeze-tts",
		"baseURL": base, "apiKeyAction": "keep", "activate": true,
	}
}

func TestBreezeModelsDefaultsPersistAndRetainZeroSeed(t *testing.T) {
	s := newTestServer(t, false)
	draft := breezeConnectionDraft("http://127.0.0.1:7860/")
	draft["seed"] = 0
	state := decodeMediaConfigResponse(t, mediaConfigRequest(t, s, "POST", "/api/media-models", draft))
	if len(state.Connections) != 1 || state.Selected["audio"] != state.SavedID {
		t.Fatalf("Breeze was not selected: %+v", state)
	}
	c := state.Connections[0]
	if c.Model != "breeze-tts-2" || c.BaseURL != "http://127.0.0.1:7860/v1" || c.Instruction == "" || c.CFGScale != 4 || c.Seed == nil || *c.Seed != 0 || c.HasAPIKey {
		t.Fatalf("Breeze defaults or explicit seed were lost: %+v", c)
	}
	restarted := newTestServer(t, false)
	restarted.configPath = s.configPath
	loaded := decodeMediaConfigResponse(t, mediaConfigRequest(t, restarted, "GET", "/api/media-models", nil))
	if len(loaded.Connections) != 1 || loaded.Connections[0].Instruction != c.Instruction || loaded.Connections[0].CFGScale != 4 || loaded.Connections[0].Seed == nil || *loaded.Connections[0].Seed != 0 {
		t.Fatalf("Breeze options did not survive restart: %+v", loaded)
	}
	// A reverse proxy prefix and an existing /v1 must survive normalization.
	draft["id"] = state.SavedID
	draft["baseURL"] = "https://tts.example/breeze/v1/"
	draft["instruction"] = "平静的旁白。\n语速适中。"
	draft["cfgScale"] = 2.5
	draft["seed"] = nil
	w := mediaConfigRequest(t, s, "POST", "/api/media-models", draft)
	state = decodeMediaConfigResponse(t, w)
	updated := state.Connections[0]
	if updated.BaseURL != "https://tts.example/breeze/v1" || updated.Instruction != draft["instruction"] || updated.CFGScale != 2.5 || updated.Seed != nil {
		t.Fatalf("Breeze custom settings were lost: %+v", updated)
	}
	var response struct {
		Connections []map[string]json.RawMessage `json:"connections"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || len(response.Connections) != 1 {
		t.Fatalf("invalid Breeze JSON response: %v", err)
	}
	seed, present := response.Connections[0]["seed"]
	if !present || string(seed) != "null" {
		t.Fatalf("random seed must be explicitly null so UI does not restore a default: present=%v value=%s", present, seed)
	}
}

func TestBreezeModelValidationLeavesExistingConfigUntouched(t *testing.T) {
	s := newTestServer(t, false)
	decodeMediaConfigResponse(t, mediaConfigRequest(t, s, "POST", "/api/media-models", mediaConnectionDraft("audio")))
	path := filepath.Join(filepath.Dir(s.configPath), "media-models.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		field string
		value any
	}{
		{"kind", "image"}, {"kind", "video"},
		{"cfgScale", 0}, {"cfgScale", -1}, {"cfgScale", 10.1},
		{"seed", -1}, {"seed", uint64(1) << 32}, {"seed", 0.5},
		{"instruction", strings.Repeat("长", 2001)}, {"instruction", "声音\x00描述"},
	} {
		t.Run(fmt.Sprintf("%02d_%s", i, tc.field), func(t *testing.T) {
			draft := breezeConnectionDraft("http://127.0.0.1:7860/v1")
			draft[tc.field] = tc.value
			w := mediaConfigRequest(t, s, "POST", "/api/media-models", draft)
			if w.Code != 400 {
				t.Fatalf("invalid Breeze setting accepted: %d %s", w.Code, w.Body.String())
			}
		})
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("invalid Breeze settings overwrote configuration: %v", err)
	}
}

func TestBreezeModelInstructionRedactsSavedCredentials(t *testing.T) {
	s := newTestServer(t, false)
	draft := breezeConnectionDraft("http://127.0.0.1:7860/v1")
	draft["apiKeyAction"], draft["apiKey"] = "replace", "breeze-private-token"
	draft["instruction"] = "旁白 breeze-private-token"
	w := mediaConfigRequest(t, s, "POST", "/api/media-models", draft)
	state := decodeMediaConfigResponse(t, w)
	if strings.Contains(w.Body.String(), "breeze-private-token") || !state.Connections[0].HasAPIKey || !strings.Contains(state.Connections[0].Instruction, "[redacted]") {
		t.Fatalf("Breeze response exposed a credential: %s", w.Body.String())
	}
	stored, err := loadMediaModelConfig(filepath.Join(filepath.Dir(s.configPath), "media-models.json"))
	if err != nil || stored.Connections[0].APIKey != "breeze-private-token" || stored.Connections[0].Instruction != draft["instruction"] {
		t.Fatalf("Breeze credential or instruction was not retained privately: %v", err)
	}
}
