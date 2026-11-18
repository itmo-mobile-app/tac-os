package dbms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"
)

const snapshotVersion = 1

type snapshot[T any] struct {
	Version int                `json:"version"`
	Tables  []snapshotTable[T] `json:"tables"`
}
type snapshotTable[T any] struct {
	Name    string           `json:"name"`
	Columns []snapshotColumn `json:"columns"`
	Rows    [][]T            `json:"rows"`
}
type snapshotColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

var snapshotTypes = [...]string{Integer: "INTEGER", Real: "REAL", Text: "TEXT"}

// OpenSnapshot loads a versioned database file. A missing file starts an empty
// database. The caller must hold exclusive access to the path for its lifetime.
func OpenSnapshot(path string) (*Database, error) {
	db := &Database{snapshotPath: path}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return db, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("snapshot %q is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tables, err := decodeSnapshot(data)
	if err != nil {
		return nil, fmt.Errorf("load snapshot %q: %w", path, err)
	}
	db.tables = tables
	return db, nil
}

func validSnapshotName(name string) bool {
	if name == "" || !letter(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !letter(name[i]) && !digit(name[i]) {
			return false
		}
	}
	return true
}

func decodeSnapshot(data []byte) (map[string]*table, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("invalid UTF-8")
	}
	var state snapshot[json.RawMessage]
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after snapshot")
	}
	if state.Version != snapshotVersion {
		return nil, fmt.Errorf("unsupported snapshot version %d", state.Version)
	}
	if state.Tables == nil {
		return nil, fmt.Errorf("missing tables array")
	}
	tables := make(map[string]*table, len(state.Tables))
	for _, saved := range state.Tables {
		if !validSnapshotName(saved.Name) || tables[saved.Name] != nil {
			return nil, fmt.Errorf("invalid or duplicate table %q", saved.Name)
		}
		if len(saved.Columns) == 0 || saved.Rows == nil {
			return nil, fmt.Errorf("missing columns or rows in table %q", saved.Name)
		}
		t := &table{columns: make([]column, len(saved.Columns)), rows: make([][]Value, len(saved.Rows))}
		names := make(map[string]bool, len(saved.Columns))
		for i, c := range saved.Columns {
			if !validSnapshotName(c.Name) || names[c.Name] {
				return nil, fmt.Errorf("invalid or duplicate column %q in table %q", c.Name, saved.Name)
			}
			names[c.Name] = true
			var typeID Type
			switch c.Type {
			case "INTEGER":
				typeID = Integer
			case "REAL":
				typeID = Real
			case "TEXT":
				typeID = Text
			default:
				return nil, fmt.Errorf("unknown type %q in table %q", c.Type, saved.Name)
			}
			t.columns[i] = column{c.Name, typeID}
		}
		for i, row := range saved.Rows {
			if len(row) != len(t.columns) {
				return nil, fmt.Errorf("wrong width of row %d in table %q", i, saved.Name)
			}
			values := make([]Value, len(row))
			for j, raw := range row {
				if bytes.Equal(raw, []byte("null")) {
					return nil, fmt.Errorf("null value in row %d of table %q", i, saved.Name)
				}
				v := Value{typeID: t.columns[j].typeID}
				var err error
				switch v.typeID {
				case Integer:
					err = json.Unmarshal(raw, &v.integer)
				case Real:
					err = json.Unmarshal(raw, &v.real)
				case Text:
					err = json.Unmarshal(raw, &v.text)
				}
				if err != nil {
					return nil, fmt.Errorf("table %q row %d column %q: %w", saved.Name, i, t.columns[j].name, err)
				}
				values[j] = v
			}
			t.rows[i] = values
		}
		tables[saved.Name] = t
	}
	return tables, nil
}

func (db *Database) encodeSnapshot(w io.Writer) error {
	names := make([]string, 0, len(db.tables))
	for name := range db.tables {
		names = append(names, name)
	}
	sort.Strings(names)
	state := snapshot[any]{Version: snapshotVersion, Tables: make([]snapshotTable[any], 0, len(names))}
	for _, name := range names {
		t := db.tables[name]
		saved := snapshotTable[any]{Name: name, Columns: make([]snapshotColumn, len(t.columns)), Rows: make([][]any, len(t.rows))}
		for i, c := range t.columns {
			saved.Columns[i] = snapshotColumn{c.name, snapshotTypes[c.typeID]}
		}
		for i, row := range t.rows {
			values := make([]any, len(row))
			for j, v := range row {
				switch v.typeID {
				case Integer:
					values[j] = v.integer
				case Real:
					values[j] = v.real
				case Text:
					if !utf8.ValidString(v.text) {
						return fmt.Errorf("invalid UTF-8 in TEXT value")
					}
					values[j] = v.text
				}
			}
			saved.Rows[i] = values
		}
		state.Tables = append(state.Tables, saved)
	}
	return json.NewEncoder(w).Encode(state)
}

// saveSnapshot reports whether rename installed the new state. If directory
// sync fails after rename, callers must still adopt the visible snapshot.
func (db *Database) saveSnapshot(path string) (bool, error) {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return false, err
	}
	defer dir.Close()
	file, err := os.CreateTemp(filepath.Dir(path), ".tacdb-snapshot-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err = db.encodeSnapshot(file); err != nil {
		return false, err
	}
	if err = file.Sync(); err != nil {
		return false, err
	}
	if err = file.Close(); err != nil {
		return false, err
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return false, err
	}
	return true, dir.Sync()
}
