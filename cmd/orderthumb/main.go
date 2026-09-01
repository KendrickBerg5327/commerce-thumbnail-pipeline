package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"

	"example.com/commerce-thumbnail-pipeline/internal/infrai"
	"example.com/commerce-thumbnail-pipeline/internal/orders"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	pipeline := orders.Pipeline{Images: infrai.New(key)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders/checkout", checkoutHandler(pipeline))
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("order thumbnail service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func checkoutHandler(pipeline orders.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart checkout"})
			return
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image is required"})
			return
		}
		defer file.Close()
		image, err := io.ReadAll(io.LimitReader(file, 10<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read image"})
			return
		}
		checkout := orders.Checkout{OrderID: r.FormValue("order_id"), CustomerID: r.FormValue("customer_id"), ReceiptEmail: r.FormValue("receipt_email"), Filename: safeFilename(header), Image: image}
		if checkout.OrderID == "" || checkout.CustomerID == "" || checkout.ReceiptEmail == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "order_id, customer_id, and receipt_email are required"})
			return
		}
		result, err := pipeline.Run(r.Context(), checkout)
		if err != nil {
			var apiErr *infrai.APIError
			if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
				writeJSON(w, apiErr.Status, map[string]string{"error": apiErr.Code})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "image processing did not complete"})
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

func safeFilename(header *multipart.FileHeader) string {
	if header.Filename == "" {
		return "product-image"
	}
	return header.Filename
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
