package server

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestProtocolWithoutSocket(t *testing.T) {
	var e executor
	for _, sql := range []string{"CREATE TABLE t (n INTEGER, r REAL, x TEXT);", "INSERT INTO t (n, r, x) VALUES (9223372036854775807, 30.0, 'Привет');", "UPDATE t SET r = 0.9 WHERE n = 9223372036854775807;"} {
		if got := string(e.execute([]byte(sql))); got != "{\"ok\":true}" {
			t.Fatalf("%s: %s", sql, got)
		}
	}
	for _, sql := range []string{"SELECT n, r, x FROM t;", "SELECT x, n, x FROM t WHERE n = 0;"} {
		p := e.execute([]byte(sql))
		if !json.Valid(p) || !bytes.Contains(p, []byte(`"columns"`)) {
			t.Fatal(string(p))
		}
		if strings.Contains(sql, "WHERE") {
			if !bytes.Contains(p, []byte(`"rows":[]`)) {
				t.Fatal(string(p))
			}
		} else if !bytes.Contains(p, []byte("9223372036854775807")) {
			t.Fatal("int64 lost")
		}
	}
	for _, tc := range []struct {
		input []byte
		code  string
	}{{[]byte(""), "BAD_REQUEST"}, {[]byte("  "), "BAD_REQUEST"}, {[]byte{255}, "BAD_REQUEST"}, {[]byte("SELECT * FROM missing;"), "SQL_ERROR"}} {
		if p := e.execute(tc.input); !bytes.Contains(p, []byte(tc.code)) {
			t.Fatal(string(p))
		}
	}
	e.execute([]byte("CREATE TABLE big (x TEXT);"))
	e.execute([]byte("INSERT INTO big (x) VALUES ('" + strings.Repeat("x", 600000) + "');"))
	if p := e.execute([]byte("SELECT x,x FROM big;")); !bytes.Contains(p, []byte("RESULT_TOO_LARGE")) || len(p) > MaxFrame {
		t.Fatal(string(p[:100]))
	}
	var framed bytes.Buffer
	if err := writeFrame(&framed, []byte("Привет")); err != nil {
		t.Fatal(err)
	}
	got, err := readFrame(&framed)
	if err != nil || string(got) != "Привет" {
		t.Fatal(string(got), err)
	}
	for _, input := range [][]byte{{0, 0, 0, 0}, {1, 0, 16, 0}, {1, 2}, {5, 0, 0, 0, 'x'}} {
		if _, err := readFrame(bytes.NewReader(input)); err == nil {
			t.Fatal("invalid frame accepted")
		}
	}
	if _, err := readFrame(bytes.NewReader(nil)); err != io.EOF {
		t.Fatal(err)
	}
}
