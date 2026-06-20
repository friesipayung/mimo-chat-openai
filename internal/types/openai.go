package types

import "encoding/json"

type ThinkingConfig struct {
	Type string `json:"type"` // "enabled" or "disabled"
}

type Tool struct {
	Type       string        `json:"type"`
	WebSearch  *WebSearchCfg `json:"web_search,omitempty"`
	Function   *FunctionCfg  `json:"function,omitempty"`
}

type WebSearchCfg struct {
	Enabled     bool `json:"enabled"`
	ForceSearch bool `json:"force_search,omitempty"`
	MaxKeyword  int  `json:"max_keyword,omitempty"`
}

type FunctionCfg struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type Annotation struct {
	Type        string `json:"type"`
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	Summary     string `json:"summary,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
	PublishTime string `json:"publish_time,omitempty"`
	LogoURL     string `json:"logo_url,omitempty"`
}

type ResponseFormat struct {
	Type string `json:"type"` // "json_object" or "text"
}

type ChatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []ChatMessage   `json:"messages"`
	Stream         bool            `json:"stream,omitempty"`
	Temperature    *float64        `json:"temperature,omitempty"`
	TopP           *float64        `json:"top_p,omitempty"`
	MaxTokens      *int            `json:"max_tokens,omitempty"`
	Thinking       *ThinkingConfig `json:"thinking,omitempty"`
	Tools          []Tool          `json:"tools,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

type ChatMessage struct {
	Role             string        `json:"role"`
	Content          interface{}   `json:"content"`
	ReasoningContent *string       `json:"reasoning_content,omitempty"`
	Annotations      []Annotation  `json:"annotations,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   *Usage       `json:"usage,omitempty"`
}

type ChatChoice struct {
	Index        int            `json:"index"`
	Message      *ChatMessage   `json:"message,omitempty"`
	Delta        *ChatDelta     `json:"delta,omitempty"`
	FinishReason *string        `json:"finish_reason,omitempty"`
}

type ChatDelta struct {
	Role             *string `json:"role,omitempty"`
	Content          *string `json:"content,omitempty"`
	ReasoningContent *string `json:"reasoning_content,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type MiMoUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelList struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
}

var SupportedModels = []Model{
	{ID: "mimo-v2.5-pro", Object: "model", Created: 1767239114, OwnedBy: "xiaomi"},
	{ID: "mimo-v2.5", Object: "model", Created: 1767239114, OwnedBy: "xiaomi"},
	{ID: "mimo-v2-flash", Object: "model", Created: 1767239114, OwnedBy: "xiaomi"},
	{ID: "mimo-v2-pro", Object: "model", Created: 1767239114, OwnedBy: "xiaomi"},
	{ID: "mimo-v2-omni", Object: "model", Created: 1767239114, OwnedBy: "xiaomi"},
}

func ModelIDToStudio(name string) string {
	m := map[string]string{
		"mimo-v2.5-pro":        "mimo-v2.5-pro",
		"mimo-v2.5":            "mimo-v2.5",
		"mimo-v2-flash":        "mimo-v2-flash-studio",
		"mimo-v2-flash-studio": "mimo-v2-flash-studio",
		"mimo-v2-pro":          "mimo-v2-pro",
		"mimo-v2-omni":         "mimo-v2-omni",
	}
	if v, ok := m[name]; ok {
		return v
	}
	return "mimo-v2.5-pro"
}

func ExtractText(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, p := range v {
			if m, ok := p.(map[string]interface{}); ok {
				if m["type"] == "text" {
					parts = append(parts, m["text"].(string))
				}
			}
		}
		result, _ := json.Marshal(parts)
		return string(result)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
