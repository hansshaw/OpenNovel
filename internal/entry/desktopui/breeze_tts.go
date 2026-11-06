package desktopui

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hansshaw/OpenNovel/internal/chaptermedia"
)

const breezeDefaultInstruction = "一位声音自然清晰的中文旁白，语速适中，语气平稳，适合有声书朗读。"
const breezeWAVHeaderBytes = 44

// The official server has one inference slot. Queue whole chapters locally so
// two projects sharing a service cannot interleave or reject each other's work.
var breezeSynthesisSlots sync.Map

func breezeBaseURL(base string) string {
	if !validMediaURL(base) {
		return base
	}
	base = strings.TrimRight(base, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base
}

func generateBreezeSpeech(ctx context.Context, c mediaModelConnection, body string, progress func(int)) ([]byte, string, error) {
	c.BaseURL = breezeBaseURL(c.BaseURL)
	slotValue, _ := breezeSynthesisSlots.LoadOrStore(c.BaseURL, make(chan struct{}, 1))
	slot := slotValue.(chan struct{})
	select {
	case slot <- struct{}{}:
		defer func() { <-slot }()
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	seed := int64(0)
	if c.Seed != nil {
		seed = *c.Seed
	} else {
		var random [4]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", errors.New("无法创建配音随机种子，请重试")
		}
		seed = int64(binary.LittleEndian.Uint32(random[:]))
	}
	instruction := c.Instruction
	if instruction == "" {
		instruction = breezeDefaultInstruction
	}
	cfgScale := c.CFGScale
	if cfgScale == 0 {
		cfgScale = 4
	}
	// Breeze caps single-request audio at 1500 frames (~120 seconds). A small
	// sentence-aware window leaves headroom for slower narration and Chinese.
	chunks := speechChunks(body, 240)
	merged := make([]byte, breezeWAVHeaderBytes)
	sampleRate := 0
	for i, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		if strings.TrimSpace(chunk) != "" {
			pcm, rate, err := breezeSpeechSegment(ctx, c, chunk, instruction, cfgScale, seed, chaptermedia.MaxGeneratedBytes-int64(len(merged)))
			if err != nil {
				return nil, "", err
			}
			if sampleRate != 0 && rate != sampleRate {
				return nil, "", errors.New("Breeze TTS 各段采样率不一致，结果未保存")
			}
			sampleRate = rate
			merged = append(merged, pcm...)
		}
		if progress != nil {
			progress((i + 1) * 95 / len(chunks))
		}
	}
	if sampleRate == 0 {
		return nil, "", errors.New("没有可朗读的正文")
	}
	// One RIFF/WAVE container around all PCM segments gives players the correct
	// duration and supports seeking without an external encoder or ffmpeg.
	copy(merged[0:4], "RIFF")
	binary.LittleEndian.PutUint32(merged[4:8], uint32(len(merged)-8))
	copy(merged[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(merged[16:20], 16)
	binary.LittleEndian.PutUint16(merged[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(merged[22:24], 1) // mono
	binary.LittleEndian.PutUint32(merged[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(merged[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(merged[32:34], 2)
	binary.LittleEndian.PutUint16(merged[34:36], 16)
	copy(merged[36:40], "data")
	binary.LittleEndian.PutUint32(merged[40:44], uint32(len(merged)-44))
	return merged, "audio/wav", nil
}

func breezeSpeechSegment(ctx context.Context, c mediaModelConnection, text, instruction string, cfgScale float64, seed int64, limit int64) ([]byte, int, error) {
	if limit < 2 {
		return nil, 0, errors.New("朗读音频超过 100 MiB 大小限制，结果未保存；请缩短章节后重试")
	}
	var form bytes.Buffer
	w := multipart.NewWriter(&form)
	for _, field := range [][2]string{
		{"text", text}, {"instruction", instruction},
		{"cfg_scale", strconv.FormatFloat(cfgScale, 'f', -1, 64)}, {"seed", strconv.FormatInt(seed, 10)},
	} {
		if err := w.WriteField(field[0], field[1]); err != nil {
			return nil, 0, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, 0, err
	}
	client := mediaHTTPClient()
	client.Timeout = 10 * time.Minute // Allow cold inference on a local GPU.
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/audio/speech", bytes.NewReader(form.Bytes()))
		if err != nil {
			return nil, 0, errors.New("Breeze TTS 服务地址无效")
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Accept", "audio/pcm")
		if c.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.APIKey)
		}
		resp, err = client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			return nil, 0, errors.New("Breeze TTS 连接失败，请确认推理服务已启动、GPU 可用且服务地址正确")
		}
		if resp.StatusCode != http.StatusConflict {
			break
		}
		resp.Body.Close()
		// Upstream releases its inference lock asynchronously after sending EOF.
		// A complete previous segment can therefore still cause a brief 409.
		// Only 409 guarantees that inference did not start; never retry uncertain
		// transport errors or failures after an audio stream has begun.
		if attempt >= 5 {
			return nil, 0, errors.New("Breeze TTS 正在处理其他配音请求，请等待完成后重试（HTTP 409）")
		}
		timer := time.NewTimer(min(100*time.Millisecond<<attempt, time.Second))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, 0, ctx.Err()
		case <-timer.C:
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Upstream errors may echo chapter text or credentials; never relay them.
		return nil, 0, fmt.Errorf("Breeze TTS 返回 HTTP %d，请检查服务状态、声音描述和接入配置", resp.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "audio/pcm" || resp.Header.Get("X-Sample-Format") != "s16le" {
		return nil, 0, errors.New("Breeze TTS 未返回 s16le PCM 音频，请使用官方推理服务接口")
	}
	rate, err := strconv.Atoi(resp.Header.Get("X-Sample-Rate"))
	if err != nil || rate < 8000 || rate > 192000 {
		return nil, 0, errors.New("Breeze TTS 返回的音频采样率无效")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, errors.New("Breeze TTS 音频传输中断，结果未保存，请重试")
	}
	if int64(len(data)) > limit {
		return nil, 0, errors.New("朗读音频超过 100 MiB 大小限制，结果未保存；请缩短章节后重试")
	}
	if len(data) == 0 || len(data)%2 != 0 {
		return nil, 0, errors.New("Breeze TTS 返回空音频或不完整的 PCM 数据，结果未保存")
	}
	return data, rate, nil
}
