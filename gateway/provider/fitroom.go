package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// FitroomProvider calls Fitroom's virtual try-on API.
// Verified against official docs on 2026-10-13 (developer.fitroom.app):
//   submit: POST https://platform.fitroom.app/api/tryon/v2/tasks (multipart)
//   auth:   X-API-KEY: <key>
//   fields: model_image (file), cloth_image (file), cloth_type=upper|lower|full_set|combo
//   status: GET https://platform.fitroom.app/api/tryon/v2/tasks/:id
//           -> {status: CREATED|PROCESSING|COMPLETED|FAILED, progress: 0-100,
//               download_signed_url (on COMPLETED)}
//   price:  1 credit / try-on ($35 per 1000 credits = $0.035/image, credits don't expire)
//   speed:  ~9s standard, ~30s HD
type FitroomProvider struct {
	APIKey     string
	Endpoint   string // default https://platform.fitroom.app
	HDMode     bool
	HTTPClient *http.Client
}

func NewFitroom(apiKey, endpoint string) *FitroomProvider {
	if endpoint == "" {
		endpoint = "https://platform.fitroom.app"
	}
	return &FitroomProvider{
		APIKey:   apiKey,
		Endpoint: strings.TrimSuffix(endpoint, "/"),
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (p *FitroomProvider) Name() string { return "fitroom" }

// clothType maps our category vocabulary to Fitroom's cloth_type.
func clothType(category string) string {
	switch category {
	case "tops", "upper":
		return "upper"
	case "bottoms", "lower":
		return "lower"
	case "one-pieces", "full_set", "dresses":
		return "full_set"
	default:
		return "upper" // most common case; "auto" has no Fitroom equivalent
	}
}

// decodeImage turns a data: URI (or http URL, passed through as error since
// Fitroom needs file bytes) into raw bytes + file extension.
func decodeImage(s string) ([]byte, string, error) {
	if !strings.HasPrefix(s, "data:") {
		return nil, "", fmt.Errorf("fitroom: needs image bytes, got URL — upload the file from the extension instead")
	}
	comma := strings.Index(s, ",")
	if comma < 0 {
		return nil, "", fmt.Errorf("fitroom: malformed data URI")
	}
	meta := s[:comma]
	raw, err := base64.StdEncoding.DecodeString(s[comma+1:])
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(s[comma+1:])
		if err != nil {
			return nil, "", fmt.Errorf("fitroom: base64 decode failed")
		}
	}
	ext := "jpg"
	switch {
	case strings.Contains(meta, "png"):
		ext = "png"
	case strings.Contains(meta, "webp"):
		ext = "webp"
	}
	return raw, ext, nil
}

func (p *FitroomProvider) Submit(ctx context.Context, req TryOnRequest) (string, error) {
	if p.APIKey == "" {
		return "", fmt.Errorf("fitroom: FITROOM_API_KEY is not set")
	}
	personBytes, personExt, err := decodeImage(req.PersonImage)
	if err != nil {
		return "", err
	}
	garmentBytes, garmentExt, err := decodeImage(req.GarmentImage)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("model_image", "model."+personExt)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(personBytes); err != nil {
		return "", err
	}
	fw, err = mw.CreateFormFile("cloth_image", "cloth."+garmentExt)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(garmentBytes); err != nil {
		return "", err
	}
	_ = mw.WriteField("cloth_type", clothType(req.Category))
	if p.HDMode {
		_ = mw.WriteField("hd_mode", "true")
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.Endpoint+"/api/tryon/v2/tasks", &body)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())
	httpReq.Header.Set("X-API-KEY", p.APIKey)

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("fitroom submit: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fitroom submit: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	// Response carries the task id; accept a few shapes defensively.
	var out struct {
		TaskID string `json:"task_id"`
		ID     string `json:"id"`
		Data   struct {
			TaskID string `json:"task_id"`
			ID     string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("fitroom submit: bad response: %s", truncate(string(respBody), 300))
	}
	taskID := out.TaskID
	if taskID == "" {
		taskID = out.ID
	}
	if taskID == "" {
		taskID = out.Data.TaskID
	}
	if taskID == "" {
		taskID = out.Data.ID
	}
	if taskID == "" {
		return "", fmt.Errorf("fitroom submit: no task id in response: %s", truncate(string(respBody), 300))
	}
	return taskID, nil
}

func (p *FitroomProvider) Poll(ctx context.Context, providerTaskID string) (bool, string, error) {
	url := p.Endpoint + "/api/tryon/v2/tasks/" + providerTaskID
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, "", err
	}
	httpReq.Header.Set("X-API-KEY", p.APIKey)

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return false, "", fmt.Errorf("fitroom poll: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, "", fmt.Errorf("fitroom poll: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	var st struct {
		Status           string `json:"status"`
		Progress         int    `json:"progress"`
		DownloadSignedURL string `json:"download_signed_url"`
		Data             struct {
			Status           string `json:"status"`
			DownloadSignedURL string `json:"download_signed_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &st); err != nil {
		return false, "", fmt.Errorf("fitroom poll: bad response: %s", truncate(string(respBody), 300))
	}
	status := st.Status
	resultURL := st.DownloadSignedURL
	if status == "" {
		status = st.Data.Status
		resultURL = st.Data.DownloadSignedURL
	}
	switch status {
	case "COMPLETED":
		if resultURL == "" {
			return false, "", fmt.Errorf("fitroom: task completed but no download url")
		}
		return true, resultURL, nil
	case "FAILED":
		return true, "", fmt.Errorf("fitroom: upstream task failed (credit not charged per docs)")
	case "CREATED", "PROCESSING":
		return false, "", nil
	default:
		return false, "", fmt.Errorf("fitroom: unknown status %q", status)
	}
}
