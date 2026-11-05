package tacticalmap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const maxBrokerFrame = 1024 * 1024

// BrokerOptions enables the existing TCP protocol, not a Broker implementation.
// Observations are a one-shot input; no delivery acknowledgement or replay exists.
type BrokerOptions struct {
	Address      string
	Observations []Observation
}

// RunWithBroker starts an empty map and updates it as Target messages arrive.
// Broker failure leaves the last frame visible until the user closes the window.
func RunWithBroker(ctx context.Context, client Client, cfg Config, options BrokerOptions, log io.Writer) error {
	if options.Address == "" {
		return fmt.Errorf("Broker address is required")
	}
	payloads := make([][]byte, 0, len(options.Observations))
	for _, observation := range options.Observations {
		if err := observation.validate(); err != nil {
			return err
		}
		payload, err := json.Marshal(observation)
		if err != nil {
			return err
		}
		if len(payload)+len("PUB observations \n") >= maxBrokerFrame {
			return fmt.Errorf("Observation exceeds the Broker line limit")
		}
		payloads = append(payloads, payload)
	}
	ctx, cancel := context.WithCancel(ctx)
	updates := make(chan Target, 16)
	notices := make(chan string, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(updates)
		defer close(notices)
		notice := func(message string) {
			select {
			case notices <- message:
			case <-ctx.Done():
			}
		}
		if err := receiveTargets(ctx, options.Address, payloads, updates, notice); err != nil && ctx.Err() == nil {
			notice(fmt.Sprintf("Broker unavailable: %v; keeping last frame, no automatic reconnect or replay", err))
		}
	}()
	defer func() {
		cancel()
		<-done
	}()
	return runMap(ctx, client, cfg, nil, updates, notices, log)
}

func receiveTargets(ctx context.Context, address string, observations [][]byte, updates chan<- Target, notice func(string)) error {
	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	write := func(line string) error {
		if err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
			return err
		}
		_, err := io.WriteString(conn, line+"\n")
		return err
	}
	if err := write("SUB targets.updated"); err != nil {
		return err
	}
	notice("Broker connected; SUB targets.updated sent (protocol has no acknowledgement)")
	for _, payload := range observations {
		if err := write("PUB observations " + string(payload)); err != nil {
			return err
		}
		notice("Observation sent to Broker (delivery is not acknowledged)")
	}
	reader := bufio.NewReaderSize(conn, maxBrokerFrame)
	for {
		line, err := reader.ReadSlice('\n')
		if err != nil {
			return fmt.Errorf("read Broker frame: %w", err)
		}
		const prefix = "MSG targets.updated "
		if !strings.HasPrefix(string(line), prefix) {
			notice("Ignored Broker message: expected MSG targets.updated")
			continue
		}
		target, err := parseTarget(line[len(prefix) : len(line)-1])
		if err != nil {
			notice("Ignored invalid Target: " + err.Error())
			continue
		}
		select {
		case updates <- target:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
