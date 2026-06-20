package mimo

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type MediaItem struct {
	MediaType  string `json:"mediaType"`
	FileUrl    string `json:"fileUrl"`
	Name       string `json:"name"`
	Size       int    `json:"size"`
	Status     string `json:"status"`
	ObjectName string `json:"objectName"`
	Url        string `json:"url"`
	TokenUsage int    `json:"tokenUsage"`
}

type UploadInfoResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ResourceId  string `json:"resourceId"`
		ResourceUrl string `json:"resourceUrl"`
		UploadUrl   string `json:"uploadUrl"`
		ObjectName  string `json:"objectName"`
	} `json:"data"`
}

type ParseResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Id         string `json:"id"`
		TokenUsage int    `json:"tokenUsage"`
	} `json:"data"`
}

// GetMimeExt returns the file extension for a MIME type
func GetMimeExt(mime string) string {
	mapping := map[string]string{
		"image/jpeg":      ".jpg",
		"image/png":       ".png",
		"image/gif":       ".gif",
		"image/webp":      ".webp",
		"image/bmp":       ".bmp",
		"audio/mpeg":      ".mp3",
		"audio/wav":       ".wav",
		"audio/flac":      ".flac",
		"audio/x-m4a":     ".m4a",
		"audio/ogg":       ".ogg",
		"video/mp4":       ".mp4",
		"video/quicktime": ".mov",
		"video/x-msvideo": ".avi",
		"video/x-ms-wmv":  ".wmv",
	}
	if ext, ok := mapping[mime]; ok {
		return ext
	}
	return ".bin"
}

// UploadFile uploads a file to MiMo FDS and returns a MediaItem
func (c *Client) UploadFile(cookie string, data []byte, fileName string, mediaType string, modelName string) (*MediaItem, error) {
	hash := md5.Sum(data)
	md5Str := hex.EncodeToString(hash[:])

	ph := ExtractPh(cookie)
	apiURL := mimoBase + "/open-apis/resource/genUploadInfo?xiaomichatbot_ph=" + url.QueryEscape(ph)

	reqBody, _ := json.Marshal(map[string]string{
		"fileName":       fileName,
		"fileContentMd5": md5Str,
	})

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "system")
	req.Header.Set("x-timeZone", "Asia/Shanghai")
	req.Header.Set("Cookie", cookie)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("genUploadInfo failed with status %d: %s", resp.StatusCode, string(b))
	}

	var uploadResp UploadInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return nil, err
	}
	if uploadResp.Code != 0 || uploadResp.Data.UploadUrl == "" {
		return nil, fmt.Errorf("genUploadInfo failed, code: %d", uploadResp.Code)
	}

	// Upload to FDS
	putReq, err := http.NewRequest("PUT", uploadResp.Data.UploadUrl, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	putReq.Header.Set("Content-Type", "application/octet-stream")
	putReq.Header.Set("Content-MD5", md5Str)

	putResp, err := c.httpClient.Do(putReq)
	if err != nil {
		return nil, err
	}
	defer putResp.Body.Close()

	if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
		putBody, _ := io.ReadAll(putResp.Body)
		return nil, fmt.Errorf("FDS PUT Error %d: %s", putResp.StatusCode, string(putBody))
	}

	// Parse and register the file
	parseURL := mimoBase + "/open-apis/resource/parse?fileUrl=" + url.QueryEscape(uploadResp.Data.ResourceUrl) +
		"&objectName=" + url.QueryEscape(uploadResp.Data.ObjectName) +
		"&model=" + url.QueryEscape(modelName) +
		"&xiaomichatbot_ph=" + url.QueryEscape(ph)

	parseReq, err := http.NewRequest("POST", parseURL, strings.NewReader(`{}`))
	if err != nil {
		return nil, err
	}
	parseReq.Header.Set("Content-Type", "application/json")
	parseReq.Header.Set("Cookie", cookie)

	finalID := uploadResp.Data.ResourceId
	tokenUsage := 0

	parseResp, err := c.httpClient.Do(parseReq)
	if err == nil {
		defer parseResp.Body.Close()
		pb, _ := io.ReadAll(parseResp.Body)
		var pr ParseResponse
		if json.Unmarshal(pb, &pr) == nil {
			if pr.Data.Id != "" {
				finalID = pr.Data.Id
			}
			tokenUsage = pr.Data.TokenUsage
		}
	}

	mediaItem := &MediaItem{
		MediaType:  mediaType,
		FileUrl:    uploadResp.Data.ResourceUrl,
		Name:       fileName,
		Size:       len(data),
		Status:     "completed",
		ObjectName: uploadResp.Data.ObjectName,
		Url:        finalID,
		TokenUsage: tokenUsage,
	}

	return mediaItem, nil
}

// ParseBase64Image parses a base64 data URL and returns the data and MIME type
func ParseBase64Image(dataURL string) ([]byte, string, error) {
	// Format: data:image/jpeg;base64,/9j/4AAQ...
	if !strings.HasPrefix(dataURL, "data:") {
		return nil, "", fmt.Errorf("not a data URL")
	}

	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid data URL format")
	}

	// Parse mime type
	mimeInfo := strings.TrimPrefix(parts[0], "data:")
	idx := strings.Index(mimeInfo, ";")
	if idx > 0 {
		mimeInfo = mimeInfo[:idx]
	}

	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, "", fmt.Errorf("base64 decode error: %w", err)
	}

	return data, mimeInfo, nil
}

// GetMediaTypeFromMime returns the media type category from MIME type
func GetMediaTypeFromMime(mime string) string {
	if strings.HasPrefix(mime, "image/") {
		return "image"
	}
	if strings.HasPrefix(mime, "audio/") {
		return "audio"
	}
	if strings.HasPrefix(mime, "video/") {
		return "video"
	}
	return "file"
}
