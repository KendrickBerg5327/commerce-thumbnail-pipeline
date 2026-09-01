package orders

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"example.com/commerce-thumbnail-pipeline/internal/infrai"
)

type fakeImages struct {
	failWidth int
	widths    []int
}

func (f *fakeImages) Upload(context.Context, string, []byte, string) (json.RawMessage, error) {
	return json.RawMessage(`{"asset":"original"}`), nil
}

func (f *fakeImages) Resize(_ context.Context, _ []byte, req infrai.ResizeRequest, _ string) (json.RawMessage, error) {
	f.widths = append(f.widths, req.Width)
	if req.Width == f.failWidth {
		return nil, errors.New("resize rejected")
	}
	return json.RawMessage(`{"asset":"thumbnail"}`), nil
}

func TestPipelineFulfillmentDecision(t *testing.T) {
	tests := []struct {
		name            string
		failWidth       int
		wantFulfillment string
		wantImages      int
		wantErr         bool
	}{
		{name: "all responsive images complete", wantFulfillment: "ready_to_pack", wantImages: 2},
		{name: "second image is incomplete", failWidth: 800, wantFulfillment: "image_processing", wantImages: 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			images := &fakeImages{failWidth: tt.failWidth}
			got, err := (Pipeline{Images: images}).Run(context.Background(), Checkout{OrderID: "ord-1042", Filename: "shoe.jpg", Image: []byte("jpeg")})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got.Fulfillment != tt.wantFulfillment || len(got.ResponsiveImage) != tt.wantImages {
				t.Fatalf("fulfillment = %q, images = %d", got.Fulfillment, len(got.ResponsiveImage))
			}
			if !tt.wantErr && got.CustomerUpdate.Status != "media_ready" {
				t.Fatalf("customer update status = %q", got.CustomerUpdate.Status)
			}
		})
	}
}
