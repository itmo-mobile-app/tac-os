# OpenRouter vision PoC (#129)

A small, **standalone Go CLI** for checking whether a selected OpenRouter
vision model can recognize a player in a consented test image. This is not
the final TAC OS Analyzer, not a separate server and not a replacement for
the ArUco detector. It does not modify any approved `spec/` contract.

## Before running

1. Obtain an OpenRouter API key and configure the account's spending limit.
2. Choose a vision model whose supported input modalities include `image`.
   OpenRouter's catalog supports filtering by image input:
   `GET https://openrouter.ai/api/v1/models?input_modalities=image`.
3. Use only a test image you own or have explicit permission to send to
   the external model provider. The image leaves the offline local network.
4. Confirm with the instructor whether cloud AI is an approved exception to
   C-8 / NFR-1. The project lead is treating OpenRouter as a candidate,
   **not as a documented exception**.

The model is configurable and deliberately **not hard-coded**. A model can
produce fluent text yet fail localization or strict JSON output; select it
using the same test cases as the alternatives.

## Running one consented image

Use Go 1.22+ and no additional Go modules. From this directory:

```sh
export OPENROUTER_API_KEY='...'
export OPENROUTER_MODEL='provider/model-with-image-input'
go run . -image /path/to/approved-test-image.jpg -send-image
```

The `-send-image` switch means the caller confirms permission for upload.
Without it the command fails **before any network request**.

Optional `-timeout 20s` (15s by default, maximum 60s) and
`-model other-provider/vision-model` override. The API key must be provided
through the environment, never in a command argument or checked-in file.
Do **not** put real keys in this README, screenshots, test logs or issues.

Possible sanitized output:

```json
{"status":"present","bbox":{"x":0.1,"y":0.2,"w":0.3,"h":0.4}}
```

This is a **format example, not a real model result**. The application
accepts only `present`, `absent` or `uncertain`. Bounding box values
are normalized to the image and can be `null` even for `present`.

On error it exits nonzero and prints a sanitized error. It does not echo
the image, raw model reply, authentication token, or detailed provider error.
One invocation performs **at most one** paid external API request; there are
no automatic retries. It sends up to 2 MiB JPEG/PNG/WebP and limits the
model output to 220 tokens as a cost *bound*, not a currency guarantee.
Rate/pricing limits still depend on the selected model/provider/account.

## Offline unit tests (no OpenRouter credentials)

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -race -count=1 ./...
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go vet ./...
test -z "$(gofmt -l .)"
```

Tests use `httptest` and a generated two-pixel PNG. They check request
shape, exact image bytes, success/negative result decoding, malformed model
responses, 401/429, timeouts and explicit image-upload consent. **Passing
these tests does not prove any real model can detect a player.** A verified
image test needs the approved external image set and an API key.

## Evaluation and handoff to the Analyzer

See [integration proposal](../../docs/vision/openrouter-integration.md)
for the proposed result schema, test matrix, separation from ArUco, failure
handling and the owner decision points.

OpenRouter reference: https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request
and https://openrouter.ai/blog/tutorials/send-image-to-llm/

## Safety and current limits

- The code never sends ArUco `id`, `distance`, player identity or GPS to a
  model. It requests **only** player presence and optional image region.
- No camera capture is implemented. The operator selects a **still image**.
  Frame ownership and synchronized detection need approval under #141/#142.
- No `.tc` ABI, live camera binding, streaming, multi-player matching,
  probability adjustment or offline vision inference is implemented here.
- `OPENROUTER_MODEL` must be selected and proven by real tests under #104;
  a name alone is not evidence that it supports images.
