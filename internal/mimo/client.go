package mimo

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const mimoBase = "https://aistudio.xiaomimimo.com"
const platformBase = "https://platform.xiaomimimo.com"

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

type MiMoRequest struct {
	MsgID          string        `json:"msgId"`
	ConversationID string        `json:"conversationId"`
	Query          string        `json:"query"`
	IsEditedQuery  bool          `json:"isEditedQuery"`
	ModelConfig    MiMoModelCfg  `json:"modelConfig"`
	MultiMedias    []interface{} `json:"multiMedias"`
}

type MiMoModelCfg struct {
	EnableThinking  bool    `json:"enableThinking"`
	WebSearchStatus string  `json:"webSearchStatus"`
	Model           string  `json:"model"`
	Temperature     float64 `json:"temperature"`
	TopP            float64 `json:"topP"`
}

type MiMoUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

func RandHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func ExtractPh(cookie string) string {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "xiaomichatbot_ph=") {
			return strings.Trim(strings.TrimPrefix(part, "xiaomichatbot_ph="), "\"")
		}
	}
	return ""
}

func ParseCookieParts(cookie string) (serviceToken, userId, ph string) {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "serviceToken=") {
			serviceToken = strings.Trim(strings.TrimPrefix(part, "serviceToken="), "\"")
		} else if strings.HasPrefix(part, "userId=") {
			userId = strings.Trim(strings.TrimPrefix(part, "userId="), "\"")
		} else if strings.HasPrefix(part, "xiaomichatbot_ph=") {
			ph = strings.Trim(strings.TrimPrefix(part, "xiaomichatbot_ph="), "\"")
		}
	}
	return
}

func BuildCookie(serviceToken, userId, ph string) string {
	return fmt.Sprintf("serviceToken=%s; userId=%s; xiaomichatbot_ph=%s", serviceToken, userId, ph)
}

func (c *Client) Chat(cookie string, req MiMoRequest) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ph := ExtractPh(cookie)
	apiURL := mimoBase + "/open-apis/bot/chat?xiaomichatbot_ph=" + ph

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept-Language", "system")
	httpReq.Header.Set("x-timeZone", "Asia/Shanghai")
	httpReq.Header.Set("Cookie", cookie)

	return c.httpClient.Do(httpReq)
}

func (c *Client) HealthCheck(cookie string) (bool, string) {
	req := MiMoRequest{
		MsgID:          RandHex(16),
		ConversationID: RandHex(16),
		Query:          "hi",
		IsEditedQuery:  false,
		ModelConfig: MiMoModelCfg{
			EnableThinking:  false,
			WebSearchStatus: "disabled",
			Model:           "mimo-v2-flash-studio",
			Temperature:     0.8,
			TopP:            0.95,
		},
		MultiMedias: []interface{}{},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return false, err.Error()
	}
	ph := ExtractPh(cookie)
	apiURL := mimoBase + "/open-apis/bot/chat?xiaomichatbot_ph=" + ph

	httpReq, _ := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept-Language", "system")
	httpReq.Header.Set("x-timeZone", "Asia/Shanghai")
	httpReq.Header.Set("Cookie", cookie)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case 200:
		return true, "valid"
	case 401, 403:
		return false, "expired"
	default:
		return false, fmt.Sprintf("status_%d", resp.StatusCode)
	}
}

func MD5(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}

type BalanceResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"message"`
	Data struct {
		Balance              string `json:"balance"`
		FrozenBalance        string `json:"frozenBalance"`
		Currency             string `json:"currency"`
		OverdraftLimit       string `json:"overdraftLimit"`
		RemainingOverdraftLimit string `json:"remainingOverdraftLimit"`
		GiftBalance          string `json:"giftBalance"`
		CashBalance          string `json:"cashBalance"`
	} `json:"data"`
}

func (c *Client) GetBalance(cookie string) (*BalanceResponse, error) {
	apiURL := platformBase + "/api/v1/balance"

	httpReq, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("Accept-Language", "en")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Referer", platformBase+"/console/balance")
	httpReq.Header.Set("x-timezone", "Asia/Jakarta")
	httpReq.Header.Set("Cookie", cookie)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var balanceResp BalanceResponse
	if err := json.Unmarshal(body, &balanceResp); err != nil {
		return nil, err
	}

	return &balanceResp, nil
}
