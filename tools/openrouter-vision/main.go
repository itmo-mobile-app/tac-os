// Command openrouter-vision sends a consented still image to a vision-capable
// model through OpenRouter. It is a PoC for #129, not the Tactical App Runtime API.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	openRouterEndpoint = "https://openrouter.ai/api/v1/chat/completions"
	maxImageBytes      = 2 << 20
	maxResponseBytes   = 128 << 10
)

const prompt = `Classify this airsoft-game camera image. Is a human player visibly present?
Reply with ONLY a JSON object with exactly these keys:
{"status":"present|absent|uncertain","bbox":null}
If a player is clearly visible, use "present". If no person is visible,
use "absent". If the image is ambiguous, use "uncertain".
For "present", bbox may be {"x":0.1,"y":0.2,"w":0.3,"h":0.4},
normalized to [0,1] and enclosing ONE visible player, or null if not
reliably localized. For "absent" and "uncertain", bbox MUST be null.
Do not include commentary or Markdown. Do not guess an ArUco marker ID,
a person's identity, distance, geographic position, or a confidence score.`

type box struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type result struct {
	Status string `json:"status"`
	BBox   *box   `json:"bbox"`
}

type completionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func decodeStrict[T any](input []byte, target *T) error {
	d := json.NewDecoder(bytes.NewReader(input))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	var remainder any
	if err := d.Decode(&remainder); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON content")
	}
	return nil
}

func parseResult(raw string) (result, error) {
	var v result
	if err := decodeStrict([]byte(strings.TrimSpace(raw)), &v); err != nil {
		return result{}, errors.New("model output is not strict result JSON")
	}
	switch v.Status {
	case "present":
		// A recognized player does not imply the model can localize them.
	case "absent", "uncertain":
		if v.BBox != nil {
			return result{}, errors.New("bbox must be null without visible player")
		}
	default:
		return result{}, errors.New("unknown model recognition status")
	}
	if v.BBox != nil {
		b := v.BBox
		for _, x := range []float64{b.X, b.Y, b.W, b.H} {
			if math.IsNaN(x) || math.IsInf(x, 0) || x < 0 || x > 1 {
				return result{}, errors.New("invalid normalized bbox")
			}
		}
		if b.W <= 0 || b.H <= 0 || b.X+b.W > 1.000000001 || b.Y+b.H > 1.000000001 {
			return result{}, errors.New("invalid bbox area")
		}
	}
	return v, nil
}

func loadImage(path string) ([]byte, string, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, "", errors.New("cannot read image path")
	}
	if !stat.Mode().IsRegular() || stat.Size() == 0 || stat.Size() > maxImageBytes {
		return nil, "", fmt.Errorf("image must be a regular file of 1..%d bytes", maxImageBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 || len(data) > maxImageBytes {
		return nil, "", errors.New("image read failed or size changed")
	}
	mime := http.DetectContentType(data)
	switch mime {
	case "image/jpeg", "image/png", "image/webp":
		return data, mime, nil
	default:
		return nil, "", errors.New("image must be JPEG, PNG or WebP")
	}
}

func recognize(ctx context.Context, client *http.Client, endpoint, apiKey, model, mime string, image []byte) (result, error) {
	if strings.TrimSpace(model) == "" || strings.TrimSpace(apiKey) == "" {
		return result{}, errors.New("model and API key are required")
	}
	if len(image) == 0 || len(image) > maxImageBytes {
		return result{}, errors.New("invalid image size")
	}
	switch mime {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return result{}, errors.New("unsupported image type")
	}
	payload := map[string]any{
		"model": model,
		"max_tokens": 220,
		"temperature": 0,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": prompt},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]string{
							"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image),
						},
					},
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return result{}, errors.New("failed to build provider request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return result{}, errors.New("failed to prepare provider request")
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		// Do not include request URLs, images or provider bodies in diagnostics.
		return result{}, errors.New("vision API unavailable or timed out")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result{}, fmt.Errorf("vision API returned HTTP %d", response.StatusCode)
	}
	reply, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(reply) > maxResponseBytes {
		return result{}, errors.New("invalid or oversized API response")
	}
	var completion completionResponse
	if err := json.Unmarshal(reply, &completion); err != nil || len(completion.Choices) == 0 {
		return result{}, errors.New("invalid API response envelope")
	}
	return parseResult(completion.Choices[0].Message.Content)
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("openrouter-vision", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	imagePath := flags.String("image", "", "path to a consented JPEG, PNG or WebP still image")
	model := flags.String("model", os.Getenv("OPENROUTER_MODEL"), "OpenRouter model slug with image input")
	timeout := flags.Duration("timeout", 15*time.Second, "request deadline (maximum 60s)")
	confirm := flags.Bool("send-image", false, "confirm permission to upload the image to an external AI provider")
	if err := flags.Parse(args); err != nil {
		return errors.New("invalid flags; use -image, -model, -timeout and -send-image")
	}
	if flags.NArg() != 0 || *imagePath == "" {
		return errors.New("provide one image with -image")
	}
	if !*confirm {
		return errors.New("upload not authorized; pass -send-image only for approved test images")
	}
	if *timeout <= 0 || *timeout > 60*time.Second {
		return errors.New("timeout must be in (0, 60s]")
	}
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return errors.New("OPENROUTER_API_KEY is missing")
	}
	data, mime, err := loadImage(*imagePath)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: *timeout}
	res, err := recognize(context.Background(), client, openRouterEndpoint, key, *model, mime, data)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(res)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		// Never print image bytes, API keys or raw provider output.
		fmt.Fprintln(os.Stderr, "openrouter-vision:", err)
		os.Exit(1)
	}
}
