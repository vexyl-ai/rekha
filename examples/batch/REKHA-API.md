# Rekha API, the facts

- Endpoint: POST https://app.rekhasvg.com/v1/trace
- Auth: header `Authorization: Bearer <key>`; the key is in the environment variable REKHA_KEY. Never print it.
- Body: multipart/form-data with a file field `image` (PNG, JPEG or GIF) and one text field per option.
- Options: `auto=true` (race four settings, keep the closest) or `preset=poster|photo|flat|bw`; `format=svg|pdf|eps|dxf`.
- Success: 200, body is the file; headers X-Rekha-Paths (path count) and X-Rekha-Millis.
- Errors are JSON {"error": "..."}: 400 bad option or image, 401 bad key, 413 too big (free plan 4 megapixels, Pro 16), 429 daily limit or too fast (Retry-After header in seconds), 503 queue full (retry after a second), 504 timeout.
- For testing, REKHA_API may override the endpoint (a local server at http://localhost:8089/v1/trace accepts any key).
