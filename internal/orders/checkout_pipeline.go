package orders

import (
	"context"
	"encoding/json"
	"fmt"

	"example.com/commerce-thumbnail-pipeline/internal/infrai"
)

type ImageProcessor interface {
	Upload(context.Context, string, []byte, string) (json.RawMessage, error)
	Resize(context.Context, []byte, infrai.ResizeRequest, string) (json.RawMessage, error)
}

type Checkout struct {
	OrderID      string
	CustomerID   string
	ReceiptEmail string
	Filename     string
	Image        []byte
}

type CustomerUpdate struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type Result struct {
	OrderID         string            `json:"order_id"`
	CheckoutStatus  string            `json:"checkout_status"`
	Fulfillment     string            `json:"fulfillment"`
	ReceiptStatus   string            `json:"receipt_status"`
	CustomerUpdate  CustomerUpdate    `json:"customer_update"`
	Original        json.RawMessage   `json:"original"`
	ResponsiveImage []json.RawMessage `json:"responsive_images"`
}

type Pipeline struct{ Images ImageProcessor }

var thumbnailSpecs = []infrai.ResizeRequest{
	{Width: 320, Height: 320, Fit: "cover", Enlarge: false, Format: "webp", Store: true},
	{Width: 800, Height: 800, Fit: "cover", Enlarge: false, Format: "webp", Store: true},
}

func (p Pipeline) Run(ctx context.Context, checkout Checkout) (Result, error) {
	result := Result{OrderID: checkout.OrderID, CheckoutStatus: "accepted", Fulfillment: "image_processing", ReceiptStatus: "recorded"}
	original, err := p.Images.Upload(ctx, checkout.Filename, checkout.Image, checkout.OrderID+":original")
	if err != nil {
		return result, fmt.Errorf("upload product image: %w", err)
	}
	result.Original = original
	for _, spec := range thumbnailSpecs {
		key := fmt.Sprintf("%s:thumbnail:%dx%d:%s", checkout.OrderID, spec.Width, spec.Height, spec.Format)
		image, err := p.Images.Resize(ctx, checkout.Image, spec, key)
		if err != nil {
			return result, fmt.Errorf("generate %dx%d thumbnail: %w", spec.Width, spec.Height, err)
		}
		result.ResponsiveImage = append(result.ResponsiveImage, image)
	}
	result.Fulfillment = "ready_to_pack"
	result.CustomerUpdate = CustomerUpdate{OrderID: checkout.OrderID, Status: "media_ready", Message: "Product images are ready for fulfillment."}
	return result, nil
}
