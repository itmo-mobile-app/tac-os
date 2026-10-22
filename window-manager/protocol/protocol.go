package protocol

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type Color struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
	A uint8 `json:"a"`
}

type Request struct {
	Command  string `json:"command"`
	WindowID string `json:"window_id,omitempty"`
	Title    string `json:"title,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`

	ImagePath string `json:"image_path,omitempty"`
	X         *int   `json:"x,omitempty"`
	Y         *int   `json:"y,omitempty"`
	Radius    int    `json:"radius,omitempty"`
	Color     *Color `json:"color,omitempty"`
}
type Response struct {
	OK       bool   `json:"ok"`
	Message  string `json:"message,omitempty"`
	Event    string `json:"event,omitempty"`
	WindowID string `json:"window_id,omitempty"`
}

func ReadRequest(r *bufio.Reader) (Request, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		if errors.Is(err, io.EOF) && len(line) > 0 {
			// Accept a final request without a newline.
		} else {
			return Request{}, err
		}
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return Request{}, fmt.Errorf("decode request: %w", err)
	}
	if req.Command == "" {
		return Request{}, errors.New("missing command")
	}
	return req, nil
}

func WriteResponse(w io.Writer, resp Response) error {
	enc := json.NewEncoder(w)
	return enc.Encode(resp)
}
