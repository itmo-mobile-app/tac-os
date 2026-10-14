// Package cli provides stdin framing and a human-readable development interface.
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/itmo-mobile-app/tac-os/server/dbms"
)

func whitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v'
}

func readStatement(input *bufio.Reader) (string, error) {
	var source strings.Builder
	quoted, hasContent := false, false
	for {
		c, err := input.ReadByte()
		if err != nil {
			if err == io.EOF && hasContent {
				return source.String(), nil
			}
			return "", err
		}
		source.WriteByte(c)
		hasContent = hasContent || !whitespace(c)
		if c == '\'' {
			if quoted {
				next, err := input.Peek(1)
				if err == nil && next[0] == '\'' {
					// Peek guarantees this byte is available.
					_, _ = input.ReadByte()
					source.WriteByte('\'')
					continue
				}
			}
			quoted = !quoted
		} else if c == ';' && !quoted {
			return source.String(), nil
		}
	}
}

func display(v dbms.Value) string {
	if n, ok := v.Integer(); ok {
		return strconv.FormatInt(n, 10)
	}
	if n, ok := v.Real(); ok {
		return strconv.FormatFloat(n, 'g', -1, 64)
	}
	s, _ := v.Text()
	escape := strings.NewReplacer("'", "''", "\\", "\\\\", "\n", "\\n", "\r", "\\r", "\t", "\\t")
	return "'" + escape.Replace(s) + "'"
}
func printResult(output io.Writer, result dbms.Result) error {
	if len(result.Columns) == 0 {
		_, err := fmt.Fprintln(output, "OK")
		return err
	}
	if _, err := fmt.Fprintln(output, strings.Join(result.Columns, "\t")); err != nil {
		return err
	}
	for _, row := range result.Rows {
		values := make([]string, len(row))
		for i, v := range row {
			values[i] = display(v)
		}
		if _, err := fmt.Fprintln(output, strings.Join(values, "\t")); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(output, "%d row(s)\n", len(result.Rows))
	return err
}

// Run executes framed statements in one database and returns the process exit code.
func Run(input io.Reader, output, diagnostics io.Writer) int {
	reader := bufio.NewReader(input)
	var database dbms.Database
	status := 0
	for {
		source, err := readStatement(reader)
		if err == io.EOF {
			return status
		}
		if err != nil {
			fmt.Fprintln(diagnostics, "ERROR: could not read standard input")
			return 1
		}
		result, err := database.Execute(source)
		if err != nil {
			var query *dbms.QueryError
			if errors.As(err, &query) {
				if _, writeErr := fmt.Fprintf(diagnostics, "ERROR at position %d: %s\n", query.Position+1, query.Message); writeErr != nil {
					return 1
				}
			} else {
				fmt.Fprintln(diagnostics, "ERROR:", err)
				return 1
			}
			status = 1
			continue
		}
		if err := printResult(output, result); err != nil {
			fmt.Fprintln(diagnostics, "ERROR: could not write standard output")
			return 1
		}
	}
}
