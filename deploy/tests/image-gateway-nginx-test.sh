#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONFIG="$ROOT_DIR/deploy/nginx/image.vote520.com.conf.example"

test -f "$CONFIG"

required_exact_paths=(
  "/v1/models"
  "/v1/sub2api/billing"
  "/v1/images/generations"
  "/v1/images/edits"
  "/v1/images/generations/async"
  "/v1/images/edits/async"
)

for path in "${required_exact_paths[@]}"; do
  grep -Fq "location = $path {" "$CONFIG"
done

grep -Fq 'location ~ ^/v1/images/tasks/[A-Za-z0-9_-]+$ {' "$CONFIG"
grep -Fq '"https://canvas.vote520.com" $http_origin;' "$CONFIG"
grep -Fq 'client_max_body_size 100m;' "$CONFIG"
grep -Fq 'add_header Cache-Control "no-store" always;' "$CONFIG"
grep -Fq 'X-Sub2api-Image-Output-Size' "$CONFIG"
grep -Fq 'X-Sub2api-Image-Resize-Filter' "$CONFIG"
grep -Fq 'proxy_hide_header Access-Control-Allow-Origin;' "$CONFIG"
grep -Fq '"$request_method $uri $server_protocol"' "$CONFIG"
grep -Fq 'location / {' "$CONFIG"
grep -Fq 'return 404;' "$CONFIG"

if grep -Eq 'location[[:space:]]+(=|~)?[[:space:]]*/v1/(chat/completions|responses|admin|users?)' "$CONFIG"; then
  echo "image gateway must not proxy text, admin, or user routes" >&2
  exit 1
fi

echo "image gateway nginx guard passed"
