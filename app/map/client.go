package tacticalmap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type Client struct {
	Socket      string
	EventSocket string
	Timeout     time.Duration
}

type response struct {
	OK       *bool  `json:"ok"`
	Message  string `json:"message"`
	Event    string `json:"event"`
	WindowID string `json:"window_id"`
}

func (c Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 5 * time.Second
}

func readResponse(reader *bufio.Reader) (response, error) {
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return response{}, fmt.Errorf("read newline-terminated WM response (limit 64 KiB): %w", err)
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return response{}, fmt.Errorf("decode WM response: %w", err)
	}
	if resp.OK == nil {
		return response{}, fmt.Errorf("WM response is missing boolean 'ok'")
	}
	if !*resp.OK {
		return response{}, fmt.Errorf("WM rejected request: %s", resp.Message)
	}
	return resp, nil
}

// Send uses one connection per command, matching the existing WM protocol.
func (c Client) Send(ctx context.Context, command Command) error {
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(requestCtx, "unix", c.Socket)
	if err != nil {
		return fmt.Errorf("%s: connect to WM %s: %w", command.Command, c.Socket, err)
	}
	defer conn.Close()
	stop := context.AfterFunc(requestCtx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := requestCtx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	if err := json.NewEncoder(conn).Encode(command); err != nil {
		return fmt.Errorf("%s: write command: %w", command.Command, err)
	}
	if _, err := readResponse(bufio.NewReaderSize(conn, 64*1024)); err != nil {
		return fmt.Errorf("%s: %w", command.Command, err)
	}
	return nil
}

type events struct {
	conn   net.Conn
	reader *bufio.Reader
	stop   func() bool
}

func (c Client) subscribe(ctx context.Context) (*events, error) {
	dialCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "unix", c.EventSocket)
	if err != nil {
		return nil, fmt.Errorf("connect to WM events %s: %w", c.EventSocket, err)
	}
	stream := &events{conn: conn, reader: bufio.NewReaderSize(conn, 64*1024)}
	// Cancel a blocked subscription/read without polling or leaving a goroutine behind.
	stream.stop = context.AfterFunc(ctx, func() { _ = conn.Close() })
	deadline, _ := dialCtx.Deadline()
	if err := conn.SetReadDeadline(deadline); err != nil {
		stream.close()
		return nil, err
	}
	resp, err := readResponse(stream.reader)
	if err != nil {
		stream.close()
		return nil, fmt.Errorf("WM event subscription: %w", err)
	}
	if resp.Event != "" || resp.Message != "subscribed" {
		stream.close()
		return nil, fmt.Errorf("expected WM event subscription acknowledgement")
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		stream.close()
		return nil, err
	}
	return stream, nil
}

func (e *events) close() {
	e.stop()
	_ = e.conn.Close()
}
