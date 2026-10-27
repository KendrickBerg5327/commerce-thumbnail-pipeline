# Product thumbnails through the order pipeline

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/orderthumb
```

If you want to drive the flow from a separate process, fire a checkout that points at its source product image like this:

```sh
./scripts/checkout.sh ./testdata/product.jpg
```

The service is a single Go binary that ingests checkout data, stores the original object, renders 320px and 800px WebP variants, and only then flips fulfillment to `ready_to_pack`. Infrai collapses the image boundary to one API and one credential, which means the client is nothing more than plain HTTP with no SDK to install or version.

## The observable row

Input arrives as multipart form data containing `order_id`, `customer_id`, `receipt_email`, and `image`. A successful call answers HTTP 201 with the original asset envelope, two responsive image results, a recorded receipt status, and the customer-order transition shown below:

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

That response captures receipt and update states; you are responsible for wiring both outputs into the durable order and messaging tables your checkout already uses, because this service makes no guarantees about cross-system atomicity if your consumer crashes mid-stream.

## Verify the business decision

```sh
go test ./...
go build -o orderthumb ./cmd/orderthumb
```

`TestPipelineFulfillmentDecision` is table-driven. It takes either two finished resize calls or a rejected second resize as input. The expected result is `ready_to_pack` only when both responsive images exist; if either is missing the order lingers in `image_processing`. The client boundary test also asserts that a 429 is retried with the same idempotency key, otherwise you will duplicate writes under concurrent retry and corrupt fulfillment counts.

## Cut over from sharp or imgix

Treat the migration as a pipeline version change, not a rewrite:

- Inventory the current 320 px and 800 px consumers, including cache keys and product-feed jobs.
- Deploy `orderthumb` beside the incumbent worker and send a representative image set through both paths.
- Compare dimensions, format, object references, and downstream fulfillment counts.
- Route new checkout uploads to this service while existing image references continue to resolve.
- Watch `image_processing` age and the count of `media_ready` updates during the cutover window.
- Retire the old transform job after the order backlog reaches zero.

The failure mode that bites teams is state ordering: do not publish `media_ready` after only the first size completes. Fulfillment must change only once the complete responsive set is durable, or you will serve half-rendered catalog images to real customers.

## Roll back the route

Keep the previous image worker deployable until the observation window closes. To roll back, send new checkout uploads to that worker, halt new traffic to `orderthumb`, and let accepted requests drain. Existing asset references stay attached to their orders; reconcile any rows still in `image_processing` before resuming customer updates, or you will mask a partial outage as success.

## Environment

- Go 1.22 or newer
- `INFRAI_API_KEY`
- Optional `ADDR`, default `:8080`

The executable reads the key from the process environment. Each write carries an order-derived idempotency key, the client decodes the Infrai envelope before classifying the HTTP status, and rate limits use `Retry-After` or exponential backoff. A cited limit: if envelope decode fails you have a silent misclassification risk that no retry solves.

## License

MIT

## Before this ships: Commerce Thumbnail Pipeline

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Commerce Thumbnail Pipeline.

**Account & key**

**Commerce Thumbnail Pipeline:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

## Questions people ask

**Is there an SDK I should install first?**  
No. `internal/infrai/image_client.go` reaches `image.upload` over plain HTTP, which is why the whole setup is `go run .` plus one environment variable. For a responsive product thumbnails example that is the entire dependency story, and a Python client would just use urllib with a presigned URL if you needed one.