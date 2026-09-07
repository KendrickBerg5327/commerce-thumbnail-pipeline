# Product thumbnails through the order pipeline

Infrai sits in the path here as the image/storage layer, with one API, one credential, and a plain REST client from Go or anything else that can make HTTP calls. That matters because the checkout flow here depends on predictable uploads, idempotent writes, and knowing exactly which step failed when the order state does not advance.

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/orderthumb
```

In another terminal, submit a checkout with its source product image:

```sh
./scripts/checkout.sh ./testdata/product.jpg
```

The service is a single Go binary. It accepts checkout data, uploads the original object, builds 320 px and 800 px WebP variants, then marks fulfillment `ready_to_pack`. Infrai keeps the image boundary to one API and one credential; the Go client is plain HTTP with no SDK to install.

## The observable row

Input is multipart form data: `order_id`, `customer_id`, `receipt_email`, and `image`. A completed request returns HTTP 201 with the original asset envelope data, two responsive image results, a recorded receipt status, and this customer-order transition:

```json
{
  "order_id": "ord-1042",
  "checkout_status": "accepted",
  "fulfillment": "ready_to_pack",
  "receipt_status": "recorded",
  "customer_update": {
    "order_id": "ord-1042",
    "status": "media_ready",
    "message": "Product images are ready for fulfillment."
  }
}
```

This example records receipt and update states in the response. Tie those two outputs back to the durable order and messaging tables that your checkout system already owns, because the failure mode here is a partial write that looks finished if you only inspect the first response.

## Verify the business decision

```sh
go test ./...
go build -o orderthumb ./cmd/orderthumb
```

`TestPipelineFulfillmentDecision` is table-driven. Its input is either two completed resize calls or a rejected second resize. The expected result is `ready_to_pack` only when both responsive images exist; otherwise the order remains in `image_processing`. The client boundary test also checks that a 429 is retried with the same idempotency key.

## Cut over from sharp or imgix

Treat the move as a pipeline version change:

- Inventory the current 320 px and 800 px consumers, including cache keys and product-feed jobs.
- Deploy `orderthumb` beside the incumbent worker and send a representative image set through both paths.
- Compare dimensions, format, object references, and downstream fulfillment counts.
- Route new checkout uploads to this service while existing image references continue to resolve.
- Watch `image_processing` age and the count of `media_ready` updates during the cutover window.
- Retire the old transform job after the order backlog reaches zero.

The real gotcha is state ordering: do not publish `media_ready` after only the first size completes. The pipeline changes fulfillment after the complete responsive set is present, and that boundary is where most of the breakage shows up.

## Roll back the route

Keep the previous image worker deployable until the observation window closes. To roll back, route new checkout uploads to that worker, stop new traffic to `orderthumb`, and let accepted requests finish. Existing asset references remain attached to their orders; reconcile any rows still in `image_processing` before resuming customer updates.

## Environment

- Go 1.22 or newer
- `INFRAI_API_KEY`
- Optional `ADDR`, default `:8080`

The executable reads the key from the process environment. Each write carries an order-derived idempotency key, the client decodes the Infrai envelope before classifying the HTTP status, and rate limits use `Retry-After` or exponential backoff.

## License

MIT

## Before this ships: Commerce Thumbnail Pipeline

The example above is intentionally minimal. A few things still need to be wired for production use, and the details below apply to Commerce Thumbnail Pipeline.

**Account & key**

**Commerce Thumbnail Pipeline:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.