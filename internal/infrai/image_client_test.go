package infrai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUploadRetriesRateLimitWithSameIdempotencyKey(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "ord-7:original" {
			t.Errorf("idempotency key = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["file"] != "anBlZw==" || body["filename"] != "shoe.jpg" || body["idempotency_key"] != "ord-7:original" {
			t.Errorf("upload body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"ok":false,"data":null,"error":{},"metadata":{}}`)
			return
		}
		fmt.Fprint(w, `{"ok":true,"data":{"asset":"stored"},"error":null,"metadata":{}}`)
	}))
	defer server.Close()

	client := New("test-key")
	client.BaseURL = server.URL
	client.Sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := client.Upload(context.Background(), "shoe.jpg", []byte("jpeg"), "ord-7:original"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestResizeUsesProcessOpsJSONShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"width", "height", "fit", "enlarge"} {
			if _, ok := body[forbidden]; ok {
				t.Errorf("forbidden top-level field %q present", forbidden)
			}
		}
		image := body["image"].(map[string]any)
		ops := body["ops"].([]any)
		op := ops[0].(map[string]any)
		params := op["params"].(map[string]any)
		if image["base64"] != "anBlZw==" || op["op"] != "resize" || params["width"] != float64(320) {
			t.Errorf("process body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"data":{"asset":"thumbnail"},"error":null,"metadata":{}}`)
	}))
	defer server.Close()

	client := New("test-key")
	client.BaseURL = server.URL
	_, err := client.Resize(context.Background(), []byte("jpeg"), ResizeRequest{
		Width: 320, Height: 320, Fit: "cover", Format: "webp", Store: true,
	}, "ord-7:thumbnail")
	if err != nil {
		t.Fatal(err)
	}
}
