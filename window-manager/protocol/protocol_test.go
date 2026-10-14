package protocol

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadRequest(t *testing.T) {
	got, err := ReadRequest(bufio.NewReader(strings.NewReader("{\"command\":\"CREATE\"}\n")))
	if err != nil { t.Fatal(err) }
	if got.Command != "CREATE" { t.Fatalf("got command %q", got.Command) }
}

func TestReadRequestRejectsMissingCommand(t *testing.T) {
	_, err := ReadRequest(bufio.NewReader(strings.NewReader("{\"title\":\"x\"}\n")))
	if err == nil { t.Fatal("expected error for missing command") }
}
