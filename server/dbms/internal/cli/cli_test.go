package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	for _, tc := range []struct {
		name, input, output, errorText string
		status                         int
	}{
		{"update", "CREATE TABLE t(x TEXT);INSERT INTO t(x)VALUES('old');UPDATE t SET x = 'O''Reilly; yes' WHERE x = 'old';SELECT * FROM t;", "OK\nOK\nOK\nx\n'O''Reilly; yes'\n1 row(s)\n", "", 0},
		{"update-recovery", "CREATE TABLE t(x INTEGER);INSERT INTO t(x)VALUES(1);UPDATE t SET x = 'wrong' WHERE x = 1;SELECT * FROM t;UPDATE t SET x = 2 WHERE x = 1;SELECT * FROM t;", "OK\nOK\nx\n1\n1 row(s)\nOK\nx\n2\n1 row(s)\n", "Type mismatch for column: x", 1},
		{"empty", " \t\r\n", "", "", 0},
		{"multiple", "CREATE TABLE t (x INTEGER);INSERT INTO t(x)VALUES(7);SELECT * FROM t;", "OK\nOK\nx\n7\n1 row(s)\n", "", 0},
		{"recovery", "CREATE TABLE t (x INTEGER);DELETE FROM t;INSERT INTO t(x)VALUES(7);SELECT * FROM t;", "OK\nOK\nx\n7\n1 row(s)\n", "ERROR at position 1:", 1},
		{"incomplete", "CREATE TABLE t (x INTEGER);SELECT * FROM t", "OK\n", "Expected ';'", 1},
		{"unclosed", "CREATE TABLE t(x TEXT);INSERT INTO t(x)VALUES('bad;", "OK\n", "Unterminated TEXT", 1},
		{"leading-space-position", "  DELETE FROM t;", "", "ERROR at position 3:", 1},
		{"escaped-quotes", "CREATE TABLE t(x TEXT);INSERT INTO t(x)VALUES('O''Reilly; yes');SELECT * FROM t;", "OK\nOK\nx\n'O''Reilly; yes'\n1 row(s)\n", "", 0},
		{"multiline", "CREATE TABLE t(x TEXT);INSERT INTO t(x)VALUES('a\nb\tc\\d');SELECT * FROM t;", "OK\nOK\nx\n'a\\nb\\tc\\\\d'\n1 row(s)\n", "", 0},
		{"empty-text", "CREATE TABLE t(x TEXT);INSERT INTO t(x)VALUES('');SELECT * FROM t;", "OK\nOK\nx\n''\n1 row(s)\n", "", 0},
		{"real-type", "CREATE TABLE t(x REAL);INSERT INTO t(x)VALUES(30.0);SELECT * FROM t;", "OK\nOK\nx\n30\n1 row(s)\n", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output, diagnostics bytes.Buffer
			status := Run(strings.NewReader(tc.input), &output, &diagnostics)
			if status != tc.status || output.String() != tc.output {
				t.Fatalf("status=%d, stdout=%q, stderr=%q", status, output.String(), diagnostics.String())
			}
			if tc.errorText == "" && diagnostics.Len() != 0 || tc.errorText != "" && !strings.Contains(diagnostics.String(), tc.errorText) {
				t.Fatalf("wrong stderr: %q", diagnostics.String())
			}
		})
	}
	// Framing must not inherit bufio.Scanner's default line limit.
	t.Run("long-text", func(t *testing.T) {
		text := strings.Repeat("a", 100000)
		input := "CREATE TABLE t(x TEXT);INSERT INTO t(x)VALUES('" + text + "');SELECT * FROM t;"
		var output, diagnostics bytes.Buffer
		if Run(strings.NewReader(input), &output, &diagnostics) != 0 || !strings.Contains(output.String(), text) {
			t.Fatalf("long input failed: %s", diagnostics.String())
		}
	})
}

type brokenIO struct{}

func (brokenIO) Read([]byte) (int, error)  { return 0, errors.New("read failed") }
func (brokenIO) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func TestIOErrors(t *testing.T) {
	var output, diagnostics bytes.Buffer
	if Run(brokenIO{}, &output, &diagnostics) != 1 {
		t.Fatal("input failure was ignored")
	}
	if Run(strings.NewReader("CREATE TABLE t(x TEXT);"), brokenIO{}, &diagnostics) != 1 {
		t.Fatal("output failure was ignored")
	}
}
