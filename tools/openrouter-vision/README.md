# OpenRouter vision PoC (#129)

A small, **standalone Go CLI** for checking whether a selected OpenRouter
vision model can detect any visible person in a consented test image —
including casually dressed participants, not just players wearing camouflage. This is not
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
export OPENROUTER_MODEL='nvidia/nemotron-3.5-super-vl:free'
go run . -image /path/to/approved-test-image.jpg -send-image -timeout 60s
```

The `-send-image` switch means the caller confirms permission for upload.
Without it the command fails **before any network request**.

The free NVIDIA model shown above is one tested option, not a built-in default.
It gets 1,024 max output tokens, `reasoning.enabled=false` and
`response_format=json_object` automatically. These settings addressed the
empty `message.content` observed during real Mac testing; no manual editing
of `main.go` is needed. Other model slugs keep the original 220-token request
without NVIDIA-specific settings.

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
One invocation performs **at most one** external API request; there are
no automatic retries. It sends up to 2 MiB JPEG/PNG/WebP and requests a
maximum of 1,024 output tokens for the free NVIDIA model, or 220 for other
models. These are token limits, **not** cost guarantees. Whether a model is
free, available or rate-limited depends on its live provider/account settings.
The CLI does not print raw model content, keys or submitted image bytes.
An empty model completion is reported as an error, never as `absent`.

## Offline unit tests (no OpenRouter credentials)

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -race -count=1 ./...
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go vet ./...
test -z "$(gofmt -l .)"
```

Tests use `httptest` and a generated two-pixel PNG. They check request
shape, exact image bytes, general-vs-NVIDIA model settings, the prompt for
ordinary people in casual clothes, success/negative result decoding, empty
completions, malformed responses, 401/402/429, timeouts and upload consent. **Passing
these tests does not prove any real model can detect a player.** A verified
image test needs the approved external image set and an API key.

## Evaluation and handoff to the Analyzer

See [integration proposal](../../docs/vision/openrouter-integration.md)
for the proposed result schema, test matrix, separation from ArUco, failure
handling and the owner decision points.

OpenRouter references: [Chat Completions API](https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request)
and [multimodal image tutorial](https://openrouter.ai/blog/tutorials/send-image-to-llm/)

## Safety and current limits

- The code never sends ArUco `id`, `distance`, player identity or GPS to a
  model. It requests **only** visible-person presence and optional image region.
- No camera capture is implemented. The operator selects a **still image**.
  Frame ownership and synchronized detection need approval under #141/#142.
- No `.tc` ABI, live camera binding, streaming, multi-player matching,
  probability adjustment or offline vision inference is implemented here.
- `OPENROUTER_MODEL` must be selected and proven by real tests under #104;
  a name alone is not evidence that it supports images.

## Committed real-image test fixtures

Seven licensed real photographs are now versioned under testdata/photos/:
three airsoft positives, one ordinary-clothes positive, and three negatives.
Their exact Git blob hashes, original sources and attribution are in
testdata/README.md and testdata/expected.json.

No fetch command or Wikimedia download is required. From this directory:

    python3 testdata/vision_samples.py check
    python3 testdata/vision_samples.py run --ids P1,C1,N1 --send-images

The first command validates local files without any network. The second
performs one external OpenRouter request per selected photo and requires
the API key and model set in the environment. Only run it when the
provider permits processing those particular photographs.

## Suggested verification images

Run approved photos of: one clearly visible casual-clothes player, a group
in everyday sportswear, a distant/partly obscured person, empty woodland,
a field with only equipment, and a humanlike mannequin. In this PoC the
mannequin false-positive is acceptable as a limitation, not proof that the
model distinguishes real people from props. Check returned bbox visually;
a valid normalized rectangle does not guarantee correct localization.

Initial operator-reported real OpenRouter testing (19 November 2026) returned
valid JSON on P1–P3 (present), N1/N2 (absent), and N3 mannequin (present),
after disabling NVIDIA reasoning locally. These were **before** the updated
casual-clothing prompt and need retesting; the images and bbox placement
have not been independently audited. See the evaluation record in
`docs/vision/openrouter-integration.md`.
