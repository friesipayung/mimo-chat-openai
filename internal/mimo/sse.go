package mimo

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/friesipayung/mimo-chat-openai/internal/types"
)

type SSEEvent struct {
	Event string
	Data  string
}

func ParseSSE(reader io.Reader, callback func(SSEEvent)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	currentEvent := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(line[6:])
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		callback(SSEEvent{Event: currentEvent, Data: data})
	}
}

func StreamToOpenAI(reader io.Reader, model string, writer io.Writer, flusher func()) error {
	chatID := "chatcmpl-" + RandHex(16)
	created := time.Now().Unix()
	role := "assistant"

	sendDelta := func(delta *types.ChatDelta, finish *string) {
		resp := types.ChatCompletionResponse{
			ID:      chatID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
			Choices: []types.ChatChoice{{
				Index:        0,
				Delta:        delta,
				FinishReason: finish,
			}},
		}
		b, _ := json.Marshal(resp)
		fmt.Fprintf(writer, "data: %s\n\n", b)
		if flusher != nil {
			flusher()
		}
	}

	sendDelta(&types.ChatDelta{Role: &role}, nil)

	inThink := false
	var usage *types.MiMoUsage
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	currentEvent := ""

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(line[6:])
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := strings.TrimSpace(line[5:])

		if currentEvent == "error" {
			var e struct {
				Content string `json:"content"`
			}
			json.Unmarshal([]byte(raw), &e)
			errMsg := e.Content
			if errMsg == "" {
				errMsg = raw
			}
			fullMsg := "Error: " + errMsg
			sendDelta(&types.ChatDelta{Content: &fullMsg}, nil)
			break
		}

		if currentEvent == "usage" {
			var u types.MiMoUsage
			if json.Unmarshal([]byte(raw), &u) == nil {
				usage = &u
			}
			continue
		}

		if currentEvent == "finish" {
			break
		}

		if currentEvent != "" && currentEvent != "message" {
			continue
		}

		var d struct {
			Content string `json:"content"`
		}
		if json.Unmarshal([]byte(raw), &d) != nil || d.Content == "" {
			continue
		}

		chunk := d.Content
		for len(chunk) > 0 {
			if !inThink {
				if i := strings.Index(chunk, "<think>"); i >= 0 {
					before := chunk[:i]
					if before != "" {
						sendDelta(&types.ChatDelta{Content: &before}, nil)
					}
					chunk = chunk[i+8:]
					inThink = true
				} else {
					if chunk != "" {
						sendDelta(&types.ChatDelta{Content: &chunk}, nil)
					}
					break
				}
			} else {
				if i := strings.Index(chunk, "</think>"); i >= 0 {
					thinkChunk := chunk[:i]
					if thinkChunk != "" {
						sendDelta(&types.ChatDelta{ReasoningContent: &thinkChunk}, nil)
					}
					chunk = chunk[i+9:]
					inThink = false
				} else {
					if chunk != "" {
						sendDelta(&types.ChatDelta{ReasoningContent: &chunk}, nil)
					}
					break
				}
			}
		}
	}

	fr := "stop"
	sendDelta(&types.ChatDelta{}, &fr)
	_ = usage
	fmt.Fprintf(writer, "data: [DONE]\n\n")
	if flusher != nil {
		flusher()
	}
	return nil
}

func CollectSync(reader io.Reader, model string) (*types.ChatCompletionResponse, error) {
	chatID := "chatcmpl-" + RandHex(16)
	created := time.Now().Unix()
	var content strings.Builder
	var inThink bool
	var usage *types.MiMoUsage

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	currentEvent := ""

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(line[6:])
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := strings.TrimSpace(line[5:])

		if currentEvent == "error" {
			var e struct {
				Content string `json:"content"`
			}
			json.Unmarshal([]byte(raw), &e)
			if e.Content != "" {
				return nil, fmt.Errorf("mimo error: %s", e.Content)
			}
			return nil, fmt.Errorf("mimo error: %s", raw)
		}

		if currentEvent == "usage" {
			var u types.MiMoUsage
			if json.Unmarshal([]byte(raw), &u) == nil {
				usage = &u
			}
			continue
		}

		if currentEvent == "finish" {
			break
		}

		if currentEvent != "" && currentEvent != "message" {
			continue
		}

		var d struct {
			Content string `json:"content"`
		}
		if json.Unmarshal([]byte(raw), &d) != nil || d.Content == "" {
			continue
		}

		chunk := d.Content
		for len(chunk) > 0 {
			if !inThink {
				if i := strings.Index(chunk, "<think>"); i >= 0 {
					content.WriteString(chunk[:i])
					chunk = chunk[i+8:]
					inThink = true
				} else {
					content.WriteString(chunk)
					break
				}
			} else {
				if i := strings.Index(chunk, "</think>"); i >= 0 {
					chunk = chunk[i+9:]
					inThink = false
				} else {
					break
				}
			}
		}
	}

	fr := "stop"
	resp := &types.ChatCompletionResponse{
		ID:      chatID,
		Object:  "chat.completion",
		Created: created,
		Model:   model,
		Choices: []types.ChatChoice{{
			Index:   0,
			Message: &types.ChatMessage{Role: "assistant", Content: content.String()},
			FinishReason: &fr,
		}},
	}

	if usage != nil {
		resp.Usage = &types.Usage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}
	}

	return resp, nil
}
