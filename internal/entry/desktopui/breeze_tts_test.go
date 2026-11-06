package desktopui

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hansshaw/OpenNovel/internal/chaptermedia"
)

func breezePCMHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "audio/pcm")
	w.Header().Set("X-Sample-Rate", "24000")
	w.Header().Set("X-Sample-Format", "s16le")
}

func breezeTestConnection(base string) mediaModelConnection {
	seed := int64(42)
	return mediaModelConnection{Kind: "audio", Protocol: "breeze-tts", BaseURL: base + "/v1", Model: "breeze-tts-2", Instruction: "清晰平稳的旁白", CFGScale: 4, Seed: &seed}
}

func assertBreezeWAV(t *testing.T, data, pcm []byte) {
	t.Helper()
	if len(data) != 44+len(pcm) || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || string(data[12:16]) != "fmt " || string(data[36:40]) != "data" {
		t.Fatalf("invalid WAV framing: %d bytes for %d PCM bytes", len(data), len(pcm))
	}
	if binary.LittleEndian.Uint32(data[4:8]) != uint32(len(data)-8) || binary.LittleEndian.Uint32(data[16:20]) != 16 || binary.LittleEndian.Uint16(data[20:22]) != 1 || binary.LittleEndian.Uint16(data[22:24]) != 1 || binary.LittleEndian.Uint32(data[24:28]) != 24000 || binary.LittleEndian.Uint32(data[28:32]) != 48000 || binary.LittleEndian.Uint16(data[32:34]) != 2 || binary.LittleEndian.Uint16(data[34:36]) != 16 || binary.LittleEndian.Uint32(data[40:44]) != uint32(len(pcm)) || !bytes.Equal(data[44:], pcm) {
		t.Fatal("WAV sample format, lengths, or concatenated PCM do not match")
	}
}

func TestBreezeChapterGenerationPreservesTextAndPersistsPlayableWAV(t *testing.T) {
	s := newTestServer(t, false)
	st := seedChapterEditor(t, s)
	body := strings.Repeat("[叹气] 夜色落在河面上。☁️🌙\nThe evening was quiet. ", 35)
	if err := st.Drafts.SaveFinalChapter(1, body); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var spoken []string
	var wantPCM []byte
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/audio/speech" || r.Header.Get("Authorization") != "Bearer breeze-proxy-token" {
			t.Errorf("unexpected Breeze request: %s %s, auth present=%v", r.Method, r.URL.Path, r.Header.Get("Authorization") != "")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("Breeze requires multipart: %v", err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		text := r.FormValue("text")
		if !utf8.ValidString(text) || utf8.RuneCountInString(text) > 240 || strings.TrimSpace(text) == "" || r.FormValue("instruction") != "沉稳的中文旁白，字句清晰。" || r.FormValue("cfg_scale") != "3.5" || r.FormValue("seed") != "0" {
			t.Errorf("Breeze options or chunk are invalid: %+v", r.MultipartForm.Value)
		}
		for _, field := range []string{"input", "model", "voice", "response_format", "instructions"} {
			if _, found := r.MultipartForm.Value[field]; found {
				t.Errorf("unsupported OpenAI/visual field sent to Breeze: %s", field)
			}
		}
		mu.Lock()
		spoken = append(spoken, text)
		pcm := []byte{byte(len(spoken)), 0, 255, 127, 0, 128}
		wantPCM = append(wantPCM, pcm...)
		mu.Unlock()
		breezePCMHeaders(w)
		w.Write(pcm)
	}))
	defer mock.Close()
	draft := breezeConnectionDraft(mock.URL)
	draft["instruction"], draft["cfgScale"], draft["seed"] = "沉稳的中文旁白，字句清晰。", 3.5, 0
	draft["apiKeyAction"], draft["apiKey"] = "replace", "breeze-proxy-token"
	decodeMediaConfigResponse(t, mediaConfigRequest(t, s, "POST", "/api/media-models", draft))
	job := createGeneration(t, s, "audio", "final")
	result := awaitGeneration(t, s, job.ID)
	if result.Status != "completed" || result.MIME != "audio/wav" || result.SourceSHA256 != generationHash(body) || result.URL == "" || result.Progress != 100 {
		t.Fatalf("Breeze generation failed: %+v", result)
	}
	mu.Lock()
	fullText, count, pcm := strings.Join(spoken, ""), len(spoken), bytes.Clone(wantPCM)
	mu.Unlock()
	if fullText != body || count < 2 {
		t.Fatalf("Breeze skipped or duplicated chapter text: %d segments", count)
	}
	w := mediaConfigRequest(t, s, "GET", result.URL, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "audio/wav" {
		t.Fatalf("WAV content not available: %d %s", w.Code, w.Body.String())
	}
	assertBreezeWAV(t, w.Body.Bytes(), pcm)
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+result.URL, nil)
	r.Header.Set("Range", "bytes=44-47")
	rw := httptest.NewRecorder()
	s.Handler().ServeHTTP(rw, r)
	if rw.Code != 206 || !bytes.Equal(rw.Body.Bytes(), pcm[:4]) || rw.Header().Get("Content-Range") == "" {
		t.Fatalf("WAV range playback failed: %d %v", rw.Code, rw.Header())
	}
	s.mu.Lock()
	s.mediaGenerations = nil
	s.mu.Unlock()
	_, stored, err := chaptermedia.ReadGeneration(s.dir, 1, result.ID)
	if err != nil || !bytes.Equal(stored, w.Body.Bytes()) {
		t.Fatalf("WAV was not durable: %v", err)
	}
	if current, err := st.Drafts.LoadChapterText(1); err != nil || current != body {
		t.Fatalf("Breeze changed the source chapter: %v", err)
	}
	deleted := mediaConfigRequest(t, s, "DELETE", "/api/chapters/1/media/generate/"+result.ID+"?directory="+url.QueryEscape(s.dir), nil)
	if deleted.Code != 200 {
		t.Fatalf("WAV deletion failed: %d %s", deleted.Code, deleted.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.dir, "media", "chapters", "000001", "generated", result.ID+".wav")); !os.IsNotExist(err) {
		t.Fatalf("WAV artifact remained after deletion: %v", err)
	}
}

func TestBreezeRejectsInvalidOrFailedPCMResponses(t *testing.T) {
	for _, mode := range []string{"busy", "unauthorized", "redirect", "html", "missing-rate", "bad-rate", "bad-format", "empty", "odd", "truncated", "rate-change"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				breezePCMHeaders(w)
				switch mode {
				case "busy":
					w.WriteHeader(409)
				case "unauthorized":
					w.WriteHeader(401)
				case "redirect":
					w.Header().Set("Location", "/unexpected")
					w.WriteHeader(307)
				case "html":
					w.Header().Set("Content-Type", "text/html")
				case "missing-rate":
					w.Header().Del("X-Sample-Rate")
				case "bad-rate":
					w.Header().Set("X-Sample-Rate", "not-a-number")
				case "bad-format":
					w.Header().Set("X-Sample-Format", "f32le")
				case "empty":
					return
				case "odd":
					w.Write([]byte{0})
					return
				case "truncated":
					w.Header().Set("Content-Length", "100")
				case "rate-change":
					if n > 1 {
						w.Header().Set("X-Sample-Rate", "48000")
					}
				}
				if mode == "busy" || mode == "unauthorized" || mode == "redirect" || mode == "html" {
					io.WriteString(w, "private-upstream-body-and-token")
				} else {
					w.Write([]byte{0, 1, 2, 3})
				}
			}))
			defer mock.Close()
			data, _, err := generateChapterMedia(context.Background(), breezeTestConnection(mock.URL), generationRequest{Kind: "audio"}, strings.Repeat("夜色渐深。", 100), func(int) {})
			if err == nil || len(data) != 0 || strings.Contains(err.Error(), "private-upstream-body-and-token") {
				t.Fatalf("invalid response accepted or leaked: data=%d error=%v", len(data), err)
			}
			if mode == "redirect" && calls.Load() != 1 {
				t.Fatalf("Breeze followed a redirect: %d requests", calls.Load())
			}
			if mode == "busy" && calls.Load() != 6 {
				t.Fatalf("Breeze busy retries were not bounded: %d requests", calls.Load())
			}
		})
	}
}

func TestBreezeRetriesPreviousStreamCleanupWithIdenticalSegment(t *testing.T) {
	var calls atomic.Int32
	var retryMu sync.Mutex
	var retryBody []byte
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if r.Header.Get("Authorization") != "Bearer proxy-token" {
			t.Error("Breeze retry lost the proxy credential")
		}
		if n == 2 {
			// The previous stream finished, but its async cleanup still owns the
			// upstream lock. No inference starts for this rejected request.
			retryMu.Lock()
			retryBody = bytes.Clone(body)
			retryMu.Unlock()
			w.WriteHeader(http.StatusConflict)
			return
		}
		if n == 3 {
			retryMu.Lock()
			identical := bytes.Equal(body, retryBody)
			retryMu.Unlock()
			if !identical {
				t.Error("Breeze retry changed or consumed the multipart request body")
			}
		}
		breezePCMHeaders(w)
		w.Write([]byte{byte(n), 0})
	}))
	defer mock.Close()
	c := breezeTestConnection(mock.URL)
	c.APIKey = "proxy-token"
	data, mime, err := generateBreezeSpeech(context.Background(), c, strings.Repeat("字", 250), nil)
	if err != nil || mime != "audio/wav" || calls.Load() != 3 {
		t.Fatalf("brief upstream cleanup rejected a segmented chapter: calls=%d mime=%s err=%v", calls.Load(), mime, err)
	}
	assertBreezeWAV(t, data, []byte{1, 0, 3, 0})
}

func TestBreezeBusyRetryWaitCanBeCancelled(t *testing.T) {
	var calls atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusConflict)
	}))
	defer mock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	data, _, err := generateBreezeSpeech(ctx, breezeTestConnection(mock.URL), "取消等待。", nil)
	if !errors.Is(err, context.DeadlineExceeded) || len(data) != 0 || calls.Load() != 1 {
		t.Fatalf("busy retry did not cancel before the next request: calls=%d bytes=%d err=%v", calls.Load(), len(data), err)
	}
}

func TestBreezeLaterSegmentFailureDoesNotSavePartialAudio(t *testing.T) {
	s := newTestServer(t, false)
	st := seedChapterEditor(t, s)
	if err := st.Drafts.SaveFinalChapter(1, strings.Repeat("一段完整的故事。", 100)); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) > 1 {
			w.WriteHeader(500)
			io.WriteString(w, "private-breeze-error")
			return
		}
		breezePCMHeaders(w)
		w.Write([]byte{0, 1})
	}))
	defer mock.Close()
	decodeMediaConfigResponse(t, mediaConfigRequest(t, s, "POST", "/api/media-models", breezeConnectionDraft(mock.URL)))
	job := createGeneration(t, s, "audio", "final")
	result := awaitGeneration(t, s, job.ID)
	if result.Status != "failed" || result.Bytes != 0 || result.MIME != "" || result.URL != "" || strings.Contains(result.Error, "private-breeze-error") {
		t.Fatalf("partial audio was published: %+v", result)
	}
	artifacts, err := filepath.Glob(filepath.Join(s.dir, "media", "chapters", "000001", "generated", "*.wav"))
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("partial WAV was saved: %v %v", artifacts, err)
	}
}

func TestBreezePCMSizeLimitIsEnforcedBeforeReturningAudio(t *testing.T) {
	var calls atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		breezePCMHeaders(w)
		w.Write([]byte{0, 1, 2, 3})
	}))
	defer mock.Close()
	c := breezeTestConnection(mock.URL)
	if data, rate, err := breezeSpeechSegment(context.Background(), c, "一句话。", c.Instruction, c.CFGScale, 42, 4); err != nil || len(data) != 4 || rate != 24000 {
		t.Fatalf("PCM exactly at the limit was rejected: data=%d rate=%d error=%v", len(data), rate, err)
	}
	if data, _, err := breezeSpeechSegment(context.Background(), c, "一句话。", c.Instruction, c.CFGScale, 42, 3); err == nil || len(data) != 0 {
		t.Fatalf("oversized PCM was returned: data=%d error=%v", len(data), err)
	}
	before := calls.Load()
	if data, _, err := breezeSpeechSegment(context.Background(), c, "一句话。", c.Instruction, c.CFGScale, 42, 1); err == nil || len(data) != 0 || calls.Load() != before {
		t.Fatalf("exhausted audio budget triggered inference: data=%d error=%v requests=%d", len(data), err, calls.Load()-before)
	}
}

func TestBreezeRandomSeedIsStableWithinChapterAndProgressAdvances(t *testing.T) {
	var seeds []string
	var mu sync.Mutex
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		seed := r.FormValue("seed")
		if _, err := strconv.ParseUint(seed, 10, 32); err != nil {
			t.Errorf("invalid randomized seed: %q", seed)
		}
		mu.Lock()
		seeds = append(seeds, seed)
		mu.Unlock()
		breezePCMHeaders(w)
		w.Write([]byte{1, 0})
	}))
	defer mock.Close()
	c := breezeTestConnection(mock.URL)
	c.Seed = nil
	var progress []int
	_, mime, err := generateChapterMedia(context.Background(), c, generationRequest{Kind: "audio"}, strings.Repeat("自然的声音。", 150), func(n int) { progress = append(progress, n) })
	if err != nil || mime != "audio/wav" {
		t.Fatalf("random-seed generation failed: %v %s", err, mime)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seeds) < 2 {
		t.Fatalf("expected a segmented chapter: %d requests", len(seeds))
	}
	for _, seed := range seeds {
		if seed != seeds[0] {
			t.Fatal("random seed changed between segments of the same chapter")
		}
	}
	if len(progress) == 0 || progress[len(progress)-1] <= 0 || progress[len(progress)-1] >= 100 {
		t.Fatalf("Breeze progress did not advance or prematurely reported a saved artifact: %v", progress)
	}
	for i := 1; i < len(progress); i++ {
		if progress[i] < progress[i-1] {
			t.Fatalf("Breeze progress moved backwards: %v", progress)
		}
	}
}

func TestBreezeSharedEndpointWaitCanBeCancelledWithoutSendingRequest(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		breezePCMHeaders(w)
		w.Write([]byte{0, 1})
	}))
	defer mock.Close()
	var once sync.Once
	unlock := func() { once.Do(func() { close(release) }) }
	defer unlock()
	c := breezeTestConnection(mock.URL)
	first := make(chan error, 1)
	go func() {
		_, _, err := generateChapterMedia(context.Background(), c, generationRequest{Kind: "audio"}, "第一章。", func(int) {})
		first <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("first Breeze request did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	data, _, err := generateChapterMedia(ctx, c, generationRequest{Kind: "audio"}, "第二章。", func(int) {})
	if !errors.Is(err, context.DeadlineExceeded) || len(data) != 0 || calls.Load() != 1 {
		t.Fatalf("queued chapter was sent or did not cancel: calls=%d bytes=%d error=%v", calls.Load(), len(data), err)
	}
	unlock()
	select {
	case err := <-first:
		if err != nil {
			t.Fatalf("first Breeze chapter failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first Breeze chapter did not finish")
	}
	if _, _, err := generateChapterMedia(context.Background(), c, generationRequest{Kind: "audio"}, "第三章。", func(int) {}); err != nil || calls.Load() != 2 {
		t.Fatalf("cancelled waiter left the Breeze endpoint locked: calls=%d error=%v", calls.Load(), err)
	}
}

func TestBreezeCancelsActivePCMStream(t *testing.T) {
	started := make(chan struct{})
	disconnected := make(chan struct{})
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		breezePCMHeaders(w)
		w.Write([]byte{0, 1})
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(disconnected)
	}))
	defer mock.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		data, _, err := generateChapterMedia(ctx, breezeTestConnection(mock.URL), generationRequest{Kind: "audio"}, "一段取消的朗读。", func(int) {})
		if len(data) != 0 {
			t.Error("cancelled PCM stream returned partial audio")
		}
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("Breeze stream did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("active stream did not preserve cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("active Breeze stream did not cancel")
	}
	select {
	case <-disconnected:
	case <-time.After(3 * time.Second):
		t.Fatal("Breeze upstream did not observe cancellation")
	}
}
