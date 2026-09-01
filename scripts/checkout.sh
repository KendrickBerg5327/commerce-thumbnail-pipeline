#!/bin/sh
set -eu

image_path="${1:?usage: scripts/checkout.sh PRODUCT_IMAGE}"

curl --fail-with-body --request POST http://localhost:8080/orders/checkout \
  --form order_id=ord-1042 \
  --form customer_id=cus-88 \
  --form receipt_email=buyer@example.com \
  --form "image=@${image_path}"
