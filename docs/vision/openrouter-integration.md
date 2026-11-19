# Proposed OpenRouter vision integration and evaluation

Status: **proposal for review**, issue #129; related #104 (Analyzer PoC),
issue #141 (Runtime integration) and issue #142 (final `.tc` Analyzer). This file is
not an approved public `spec/` contract and does not change the current
`DETECT` wire format or the six-field `Observation`.

## Goal and evaluation boundary

Show **real** model-based visual detection of any visible person, including
ordinary players wearing casual clothes, T-shirts or sportswear without
uniforms or airsoft markers. The chosen model receives an authorized still
image and may state whether a person is visible and optionally provide a
region within the image.
This is intentionally **not** automatic personal identification, ArUco
marker decoding, distance estimation, geographic localization or proof of
target identity.

The exact model remains a test-driven decision between the project lead
@TheGeniusOfEternity and Analyzer owner @slutswarming. Any OpenRouter model
with verified image input may be considered; there is no universal
"approved model" merely because it is available in the model catalog.

## External API interaction (implemented PoC)

The CLI `tools/openrouter-vision/` sends a single
`POST https://openrouter.ai/api/v1/chat/completions` request:

- `Authorization: Bearer $OPENROUTER_API_KEY` (environment only);
- `model` from `OPENROUTER_MODEL` or explicit CLI argument;
- one `messages[0].content` array with `text` and
  `image_url.url = data:<mime>;base64,<bytes>`;
- `temperature = 0` and `max_tokens = 220` by default; for the
  tested free NVIDIA Nemotron 3.5 Super VL endpoint, the CLI requests
  `max_tokens = 1024`, `reasoning.enabled = false` and
  `response_format.type = "json_object"` to avoid empty completions.
  Other models do not automatically receive the NVIDIA-specific fields.
  These settings do not promise deterministic behavior or fixed costs.

The CLI requires an explicit `-send-image` consent switch; local input
is a regular JPEG/PNG/WebP file up to 2 MiB, timeout is 15 seconds by
default (max 60s), and there is no automatic retry. Every failed or
invalid response is treated as a failure, **not** as "no player".

OpenRouter documents the multimodal Chat Completions input shape at:
[OpenRouter multimodal image tutorial](https://openrouter.ai/blog/tutorials/send-image-to-llm/)

## Candidate **result schema** for Analyzer/Runtime review

This is the **validated PoC output**, not yet a final `.tc` ABI:

```json
{"status":"present","bbox":{"x":0.10,"y":0.20,"w":0.30,"h":0.40}}
```

| Field | Meaning |
| --- | --- |
| `status` | Required enum `present` / `absent` / `uncertain` |
| `bbox` | Required field, `null` unless status is `present`; for `present` it may still be `null` |
| `bbox.x/y/w/h` | If present: finite normalized image coordinates, `w/h > 0`, rectangle within the image bounds |

Extra keys, text outside the JSON object, unexpected enum values, impossible
boxes, missing completion content or provider errors are rejected. The
PoC returns only the normalized validated result to stdout; it does
not dump raw model answers or image bytes.

**No model-generated confidence enters `Observation.confidence`.**
The current Analyzer confidence is a geometric heuristic in
`spec/analyzer-observation.md`, and the aggregated Target probability
belongs to Target Service. A `present` result is *not sufficient* to
construct an Observation if the ArUco ID/angle/distance is absent.

## Ownership and separation of concerns

| Owner | Responsibility |
| --- | --- |
| @TheGeniusOfEternity (#129) | OpenRouter account/configuration, PoC CLI, cost/privacy constraints, proposed handoff and agreement with Analyzer |
| @slutswarming (#104, #142) | Evaluate real image examples, define acceptance, consume approved AI result in the `.tc` Analyzer, own `Observation` coordinates |
| @pateWY (#141) | Decide and implement the **new** VM-facing camera/vision Runtime API, error semantics and ownership of frame capture |
| @m4shustik-s / @pypynyaa | Approve any new `.tc` return type / ABI before compiler or VM implementation |
| Instructor | Resolve explicitly whether the cloud AI demo is an allowed exception to NFR-1/C-8 |

**Possible future flow for review** (not yet selected):

```text
Linux camera frame / authorized test fixture
     ├── existing private Go Marker Detector → ArUco Detection (id, angle, distance)
     └── separate vision adapter → AI result (status, optional image bbox)
                 ↓  (new, reviewed Runtime interface)
         .tc Analyzer validates and combines *separate* inputs
                 ↓
         Observation through existing Runtime IPC → .tc Map
```

The current `camera.detectMarkers() -> String` and private
`DETECT` protocol have no frame payload and **must remain unchanged**.
It is still undecided whether Runtime captures a frame once for both
consumers or owns a separate image source. Two independently captured
frames may not be temporally aligned; do not claim exact association
of AI and ArUco without a decision and measurements.

The approved six-field `Observation` and five-field `Target` schemas
remain unchanged. The AI result should not be written into Broker/DBMS
as an extra field without a separate contract.

## Real-image evaluation protocol and initial operator results

Use the **same authorized images** for every candidate vision model.
Record only fixture IDs, expected labels, normalized result, latency,
HTTP status and observed provider cost (if supplied), **not the image or
raw model text**. The file paths/rights and test results should be kept
in the local evaluation sheet, not committed unless redistribution is
explicitly permitted.

| Case ID | Ground truth | Purpose |
| --- | --- | --- |
| P1 | One visible player with marker | Positive detection |
| P2 | Player far away / small in frame | Weak positive |
| P3 | Partially occluded player | Ambiguous or positive depending on agreed annotation |
| N1 | Empty field or background | Negative detection |
| N2 | Marker/equipment with no person | Prevent hallucinated player |
| N3 | Person-like silhouette / background object | False positive control |

For each candidate record:
`model slug`, date, provider, fixture ID, expected status, returned status,
bbox if any, format validity, timeout/HTTP errors, latency and approximate
cost. Evaluate class errors **and** inability to return strict JSON; do not
choose a model from a single positive example. A provisional gate is:
all six requests complete; no malformed results; zero false positives
on N1/N2; at least P1 succeeds. The final accuracy threshold and P3's
label are for @slutswarming and the project lead to approve.

**Initial operator-reported results (19 November 2026):** a local Mac user
tested `nvidia/nemotron-3.5-super-vl:free` via the Go CLI after adding
`reasoning.enabled=false` and expanding the output token limit. The
following statuses and normalized boxes were reported directly in the
project conversation; the photos themselves were not committed or
independently inspected, and bbox placement has not been audited.

| Image | Result status | Returned bbox (x, y, w, h) | Observation |
| --- | --- | --- | --- |
| P1 | `present` | 0.29, 0.43, 0.51, 0.22 | Positive sample |
| P2 | `present` | 0.03, 0.46, 0.25, 0.50 | Positive sample |
| P3 | `present` | 0.10, 0.15, 0.75, 0.85 | Positive sample |
| N1 | `absent` | null | Empty woodland/background |
| N2 | `absent` | null | Equipment without a person |
| N3 | `present` | 0.18, 0.03, 0.52, 0.97 | Mannequin; project coordinator accepts this false positive for initial PoC |

All six reported requests returned strictly parsable result JSON. This is
**not a measured accuracy rate** or proof of the quality of the returned boxes.
The results were obtained with the former airsoft-oriented prompt, **before**
the updated casual-clothing detection wording; re-run the model on
authorized photos of people in everyday clothing. A mannequin result
must not be confused with proof of a living person's identity.

## Errors and offline behavior

- Provider 401/403, rate limit 429, 5xx, timeout, network unavailable,
  invalid JSON and absent image are distinct operational failure classes.
  The CLI emits sanitized errors and exits nonzero, never `status=absent`
  for a failed request.
- Actual OpenRouter recognition **requires internet**. When offline,
  existing ArUco detection can continue to produce observations, but
  does not satisfy the required **AI demonstration** by itself.
- Account budget/price cap must be configured outside code; the PoC
  limits each invocation to one request and the requested output tokens,
  but cannot guarantee a fixed price across models/providers.
- Do not log API keys, raw model replies, private images, GPS, user IDs
  or provider payloads. Only consented test images may be uploaded.

## Decisions blocking completion

- [ ] Instructor explicitly confirms OpenRouter exception to NFR-1/C-8
      (or selects a locally running AI model).
- [ ] @slutswarming and @TheGeniusOfEternity approve image test set,
      expected labels and measured model selection.
- [ ] @pateWY agrees which process supplies frame bytes, how the model
      result crosses into `.tc`, timeout/error representation and whether
      image frames synchronize with ArUco detections.
- [ ] Compiler/VM owners approve a concrete ABI if Runtime needs a
      new return type.
- [x] Operator reported one set of six real image requests and negative
      controls using their own API key; resulting statuses are recorded above.
      Updated-prompt validation on casual-clothes photos and independent
      assessment of bbox placement remain incomplete.

Until then #129 provides a tested **transport/response-format prototype
by design**, not evidence of production-ready camera integration.
