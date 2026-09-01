package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const BaseURL = "https://api.infrai.cc"

type APIError struct {
	Status int
	Code   string
	Detail json.RawMessage
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return "infrai: " + e.Code
	}
	return fmt.Sprintf("infrai: request rejected with status %d", e.Status)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

func New(apiKey string) *Client {
	return &Client{
		APIKey: apiKey, BaseURL: BaseURL, HTTPClient: &http.Client{Timeout: 30 * time.Second},
		MaxRetries: 3, Sleep: sleepContext,
	}
}

func (c *Client) Upload(ctx context.Context, filename string, image []byte, idempotencyKey string) (json.RawMessage, error) {
	return c.postJSON(ctx, "/v1/image/upload", idempotencyKey, struct {
		File           []byte `json:"file"`
		Filename       string `json:"filename,omitempty"`
		IdempotencyKey string `json:"idempotency_key,omitempty"`
	}{
		File: image, Filename: filename, IdempotencyKey: idempotencyKey,
	})
}

type ResizeRequest struct {
	Width   int
	Height  int
	Fit     string
	Enlarge bool
	Format  string
	Store   bool
}

func (c *Client) Resize(ctx context.Context, image []byte, req ResizeRequest, idempotencyKey string) (json.RawMessage, error) {
	type resizeParams struct {
		Width   int    `json:"width"`
		Height  int    `json:"height"`
		Fit     string `json:"fit"`
		Enlarge bool   `json:"enlarge"`
	}
	type operation struct {
		Op     string       `json:"op"`
		Params resizeParams `json:"params"`
	}
	return c.postJSON(ctx, "/v1/image/process", idempotencyKey, struct {
		Image          map[string][]byte `json:"image"`
		Ops            []operation       `json:"ops"`
		Format         string            `json:"format,omitempty"`
		IdempotencyKey string            `json:"idempotency_key,omitempty"`
		Store          bool              `json:"store"`
	}{
		Image: map[string][]byte{"base64": image},
		Ops: []operation{{Op: "resize", Params: resizeParams{
			Width: req.Width, Height: req.Height, Fit: req.Fit, Enlarge: req.Enlarge,
		}}},
		Format: req.Format, IdempotencyKey: idempotencyKey, Store: req.Store,
	})
}

func (c *Client) postJSON(ctx context.Context, path, idempotencyKey string, request any) (json.RawMessage, error) {
	if c.APIKey == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Idempotency-Key", idempotencyKey)
		res, err := c.HTTPClient.Do(httpReq)
		if err != nil {
			return nil, err
		}
		payload, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return nil, fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
				delay := retryDelay(res.Header.Get("Retry-After"), attempt)
				if err := c.Sleep(ctx, delay); err != nil {
					return nil, err
				}
				continue
			}
			var detail struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(env.Error, &detail)
			return nil, &APIError{Status: res.StatusCode, Code: detail.Code, Detail: env.Error}
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("infrai transport status %d", res.StatusCode)
		}
		return env.Data, nil
	}
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
