package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FalAIProvider calls fal.ai queue API for the FASHN virtual try-on model.
// Verified against fal.ai official docs on 2026-10-13:
//   model: fal-ai/fashn/tryon/v1.6  ($0.075 / generation)
//   submit: POST https://queue.fal.run/fal-ai/fashn/tryon/v1.6
//   auth:   Authorization: Key $FAL_KEY
//   inputs: model_image, garment_image (URL or base64 data URI), category
//   status: GET .../requests/{request_id}/status  -> {status: IN_QUEUE|IN_PROGRESS|COMPLETED|FAILED}
//   result: GET .../requests/{request_id}         -> {images:[{url}]}
type FalAIProvider struct {
	APIKey     string
	ModelID    string // e.g. "fal-ai/fashn/tryon/v1.6"
	HTTPClient *http.Client
}

func NewFalAI(apiKey string) *FalAIProvider {
	return &FalAIProvider{
		APIKey:  apiKey,
		ModelID: "fal-ai/fashn/tryon/v1.6",
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (p *FalAIProvider) Name() string { return "falai" }

func (p *FalAIProvider) baseURL() string {
	return "https://queue.fal.run/" + p.ModelID
}

func (p *FalAIProvider) Submit(ctx context.Context, req TryOnRequest) (string, error) {
	if p.APIKey == "" {
		return "", fmt.Errorf("falai: FALAI_API_KEY is not set")
	}
	category := req.Category
	switch category {
	case "tops", "bottoms", "one-pieces", "auto":
	default:
		category = "auto"
	}
	body, _ := json.Marshal(map[string]any{
		"model_image":   req.PersonImage,
		"garment_image": req.GarmentImage,
		"category":      category,
		"mode":          "balanced",
		"num_samples":   1,
		"output_format": "jpeg",
	})
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Key "+p.APIKey)

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("falai submit: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("falai submit: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	var out struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil || out.RequestID == "" {
		return "", fmt.Errorf("falai submit: bad response: %s", truncate(string(respBody), 300))
	}
	return out.RequestID, nil
}

func (p *FalAIProvider) Poll(ctx context.Context, providerTaskID string) (bool, string, error) {
	statusURL := p.baseURL() + "/requests/" + providerTaskID + "/status"
	httpReq, err := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
	if err != nil {
		return false, "", err
	}
	httpReq.Header.Set("Authorization", "Key "+p.APIKey)

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return false, "", fmt.Errorf("falai poll: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, "", fmt.Errorf("falai poll: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	var st struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &st); err != nil {
		return false, "", fmt.Errorf("falai poll: bad status response: %s", truncate(string(respBody), 300))
	}
	switch st.Status {
	case "COMPLETED":
		url, err := p.fetchResult(ctx, providerTaskID)
		if err != nil {
			return false, "", err
		}
		return true, url, nil
	case "FAILED":
		return true, "", fmt.Errorf("falai: upstream job failed")
	case "IN_QUEUE", "IN_PROGRESS":
		return false, "", nil
	default:
		return false, "", fmt.Errorf("falai: unknown status %q", st.Status)
	}
}

func (p *FalAIProvider) fetchResult(ctx context.Context, providerTaskID string) (string, error) {
	resultURL := p.baseURL() + "/requests/" + providerTaskID
	httpReq, err := http.NewRequestWithContext(ctx, "GET", resultURL, nil)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Key "+p.APIKey)

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("falai result: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("falai result: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	var out struct {
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil || len(out.Images) == 0 || out.Images[0].URL == "" {
		return "", fmt.Errorf("falai result: no image url in response")
	}
	return out.Images[0].URL, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
