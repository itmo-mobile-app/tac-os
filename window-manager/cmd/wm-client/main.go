package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/itmo-mobile-app/tac-os/window-manager/protocol"
)

func main() {
	socket := flag.String("socket", "/tmp/tacos-wm.sock", "Unix socket path")
	title := flag.String("title", "Tactical Map", "window title")
	width := flag.Int("width", 960, "window width")
	height := flag.Int("height", 640, "window height")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: wm-client [-socket path] CREATE|SHOW|CLOSE|QUIT")
		os.Exit(2)
	}
	req := protocol.Request{Command:strings.ToUpper(flag.Arg(0)), WindowID:"map", Title:*title, Width:*width, Height:*height}
	conn, err := net.DialTimeout("unix", *socket, 2*time.Second)
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	var resp protocol.Response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	fmt.Printf("ok=%t message=%s\n", resp.OK, resp.Message)
	if !resp.OK { os.Exit(1) }
}
