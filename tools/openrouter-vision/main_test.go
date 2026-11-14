package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestVisionRequestAndResult(t *testing.T) {
	data := testPNG(t)
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected method or bearer token")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing JSON content type")
		}
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("invalid request: %v", err)
		}
		if request.Model != "test/vision-model" || len(request.Messages) != 1 || len(request.Messages[0].Content) != 2 {
			t.Errorf("wrong model or content parts")
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		parts := request.Messages[0].Content
		if parts[0].Type != "text" || !strings.Contains(parts[0].Text, "Do not guess an ArUco marker ID") {
			t.Errorf("expected constrained text prompt")
		}
		if parts[1].Type != "image_url" {
			t.Errorf("expected image_url")
		}
		prefix := "data:image/png;base64,"
		if !strings.HasPrefix(parts[1].ImageURL.URL, prefix) {
			t.Errorf("missing PNG data URL")
		} else {
			got, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(parts[1].ImageURL.URL, prefix))
			if err != nil || !bytes.Equal(got, data) {
				t.Errorf("image bytes changed")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"status\":\"present\",\"bbox\":{\"x\":0.1,\"y\":0.2,\"w\":0.3,\"h\":0.4}}"}}]}`))
	}))
	defer server.Close()

	got, err := recognize(context.Background(), server.Client(), server.URL,
		"test-key", "test/vision-model", "image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	if called.Load() != 1 || got.Status != "present" || got.BBox == nil || got.BBox.W != 0.3 {
		t.Fatalf("unexpected result: %#v; calls=%d", got, called.Load())
	}
}

func TestModelResultValidation(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"no-player", `{"status":"absent","bbox":null}`, true},
		{"missing-bbox", `{"status":"absent"}`, false},
		{"missing-bbox-coordinate", `{"status":"present","bbox":{"x":0.2,"y":0.2,"w":0.5}}`, false},
		{"uncertain", `{"status":"uncertain","bbox":null}`, true},
		{"present-without-localization", `{"status":"present","bbox":null}`, true},
		{"valid-bbox", `{"status":"present","bbox":{"x":0,"y":0,"w":1,"h":1}}`, true},
		{"unknown-status", `{"status":"enemy","bbox":null}`, false},
		{"unknown-field", `{"status":"present","bbox":null,"targetId":"17"}`, false},
		{"bbox-when-absent", `{"status":"absent","bbox":{"x":0,"y":0,"w":1,"h":1}}`, false},
		{"overflow", `{"status":"present","bbox":{"x":0.9,"y":0.1,"w":0.4,"h":0.3}}`, false},
		{"zero-width", `{"status":"present","bbox":{"x":0.2,"y":0.2,"w":0,"h":0.3}}`, false},
		{"markdown-fence", "```json\n"+`{"status":"absent","bbox":null}`+"\n```", false},
		{"extra-object", `{"status":"absent","bbox":null}{}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseResult(tc.raw)
			if (err == nil) != tc.ok {
				t.Fatalf("parseResult(%q) error=%v, want success=%v", tc.raw, err, tc.ok)
			}
		})
	}
}

func TestProviderErrorsAreSanitized(t *testing.T) {
	cases := []struct {
		name string
		code int
		body string
	}{
		{"rate-limited", http.StatusTooManyRequests, `{"error":"sensitive image data"}`},
		{"unauthorized", http.StatusUnauthorized, `{"error":"test-key"}`},
		{"bad-envelope", http.StatusOK, `{"choices":[]}`},
		{"bad-model-text", http.StatusOK, `{"choices":[{"message":{"content":"person maybe"}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := recognize(context.Background(), server.Client(), server.URL,
				"test-key", "test/vision-model", "image/png", testPNG(t))
			if err == nil || strings.Contains(err.Error(), "test-key") ||
				strings.Contains(err.Error(), "sensitive image data") {
				t.Fatalf("error missing or leaking sensitive data: %v", err)
			}
		})
	}
}

func TestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := &http.Client{Timeout: 15 * time.Millisecond}
	_, err := recognize(context.Background(), client, server.URL,
		"key", "test/vision-model", "image/png", testPNG(t))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected sanitized timeout, got %v", err)
	}
}

func TestConsentAndLocalImageValidation(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "unit-test-key")
	t.Setenv("OPENROUTER_MODEL", "test/vision-model")
	path := filepath.Join(t.TempDir(), "frame.png")
	if err := os.WriteFile(path, testPNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"-image", path}, &out); err == nil ||
		!strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("expected upload consent enforcement: %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("unauthorized call wrote output")
	}
	badPath := filepath.Join(t.TempDir(), "invalid.png")
	if err := os.WriteFile(badPath, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadImage(badPath); err == nil {
		t.Fatal("expected image validation error")
	}
	if _, _, err := loadImage(path); err != nil {
		t.Fatalf("valid PNG rejected: %v", err)
	}
}
