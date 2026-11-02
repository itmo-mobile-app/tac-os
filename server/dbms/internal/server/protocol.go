// Package server implements the approved DBMS Unix-socket protocol.
package server

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/itmo-mobile-app/tac-os/server/dbms"
)

const MaxFrame = 1048576

type column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type failure struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Position *int   `json:"position,omitempty"`
}
type response struct {
	OK      bool      `json:"ok"`
	Columns *[]column `json:"columns,omitempty"`
	Rows    *[][]any  `json:"rows,omitempty"`
	Error   *failure  `json:"error,omitempty"`
}

func errorResponse(code, message string) response {
	return response{Error: &failure{Code: code, Message: message}}
}

func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(header[:])
	if n == 0 || n > MaxFrame {
		return nil, fmt.Errorf("invalid frame length: %d", n)
	}
	data := make([]byte, int(n))
	_, err := io.ReadFull(r, data)
	return data, err
}
func writeFrame(w io.Writer, data []byte) error {
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(data)))
	for _, part := range [][]byte{header[:], data} {
		for len(part) > 0 {
			n, err := w.Write(part)
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			part = part[n:]
		}
	}
	return nil
}

type executor struct {
	mu sync.Mutex
	db dbms.Database
}

func (e *executor) execute(data []byte) []byte {
	r := errorResponse("BAD_REQUEST", "SQL payload is empty or invalid UTF-8")
	if utf8.Valid(data) && strings.TrimSpace(string(data)) != "" {
		// Serialize parsing, execution and materialization against the one shared DB.
		e.mu.Lock()
		r = e.query(string(data))
		e.mu.Unlock()
	}
	payload, err := json.Marshal(r)
	if err != nil {
		payload, _ = json.Marshal(errorResponse("INTERNAL_ERROR", "Could not encode result"))
	}
	if len(payload) > MaxFrame {
		payload, _ = json.Marshal(errorResponse("RESULT_TOO_LARGE", "Query result exceeds frame limit"))
	}
	return payload
}
func (e *executor) query(sql string) response {
	result, err := e.db.Execute(sql)
	if err != nil {
		r := errorResponse("INTERNAL_ERROR", "Could not execute query")
		var query *dbms.QueryError
		if errors.As(err, &query) {
			r.Error = &failure{Code: "SQL_ERROR", Message: query.Message, Position: &query.Position}
		}
		return r
	}
	r := response{OK: true}
	if len(result.Columns) == 0 {
		return r
	}
	columns := make([]column, len(result.Columns))
	for i, name := range result.Columns {
		columns[i] = column{name, []string{"INTEGER", "REAL", "TEXT"}[result.Types[i]]}
	}
	rows := make([][]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		values := make([]any, len(row))
		for i, v := range row {
			switch v.Type() {
			case dbms.Integer:
				values[i], _ = v.Integer()
			case dbms.Real:
				values[i], _ = v.Real()
			case dbms.Text:
				values[i], _ = v.Text()
			}
		}
		rows = append(rows, values)
	}
	r.Columns, r.Rows = &columns, &rows
	return r
}
