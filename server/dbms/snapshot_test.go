package dbms

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSnapshotRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	db, err := OpenSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"CREATE TABLE empty (n INTEGER);",
		"CREATE TABLE items (id INTEGER, score REAL, label TEXT);",
		"INSERT INTO items (id, score, label) VALUES (9223372036854775807, -0.0, 'Привет; O''Reilly');",
		"INSERT INTO items (id, score, label) VALUES (-9223372036854775808, 0.5, '');",
		"UPDATE items SET score = 0.75 WHERE id = -9223372036854775808;",
	} {
		execute(t, db, sql)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Execute("UPDATE items SET label = 'changed', score = 30 WHERE id = -9223372036854775808;")
	var query *QueryError
	if !errors.As(err, &query) {
		t.Fatalf("expected SQL error, got %v", err)
	}
	reopened, err := OpenSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"SELECT * FROM items;", "SELECT * FROM empty;", "SELECT id,score FROM items WHERE id = 9223372036854775807;"} {
		want := execute(t, db, sql)
		got := execute(t, reopened, sql)
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("%s: got %+v; want %+v", sql, got, want)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed SQL or SELECT changed snapshot")
	}
}

func TestSnapshotRejectsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	for _, data := range []string{
		"", `{"version":1,"tables":[`, `{"version":2,"tables":[]}`, `{"version":1}`, `{"version":1,"tables":[]} {}`,
		`{"version":1,"tables":[{"name":"t","columns":[{"name":"x","type":"INTEGER"}],"rows":[[null]]}]}`,
		`{"version":1,"tables":[{"name":"t","columns":[{"name":"x","type":"INTEGER"}],"rows":[[9223372036854775808]]}]}`,
		`{"version":1,"tables":[{"name":"t","columns":[{"name":"x","type":"REAL"}],"rows":[[1e999]]}]}`,
		`{"version":1,"tables":[{"name":"t","columns":[{"name":"x","type":"TEXT"}],"rows":[[1]]}]}`,
		`{"version":1,"tables":[{"name":"t","columns":[{"name":"x","type":"TEXT"}],"rows":[[]]}]}`,
		`{"version":1,"tables":[{"name":"bad name","columns":[{"name":"x","type":"TEXT"}],"rows":[]}]}`,
		`{"version":1,"tables":[{"name":"t","columns":[{"name":"x","type":"TEXT"},{"name":"x","type":"TEXT"}],"rows":[]}]}`,
		"\xff",
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenSnapshot(path); err == nil {
			t.Fatalf("accepted corrupt data %q", data)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != data {
			t.Fatalf("modified rejected snapshot %q", data)
		}
	}
}

func TestSnapshotFailedRenameDoesNotApplyWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	db, err := OpenSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	execute(t, db, "CREATE TABLE t (id INTEGER, score REAL);")
	execute(t, db, "INSERT INTO t (id, score) VALUES (1, 0.5);")
	before := execute(t, db, "SELECT * FROM t;")
	// Keep the last committed file and force rename to fail even as root.
	prior := path + ".prior"
	if err := os.Rename(path, prior); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"CREATE TABLE broken (id INTEGER);", "INSERT INTO t (id, score) VALUES (2, 0.25);", "UPDATE t SET score = 0.9 WHERE id = 1;"} {
		if _, err := db.Execute(sql); err == nil {
			t.Fatalf("write succeeded: %s", sql)
		}
	}
	if got := execute(t, db, "SELECT * FROM t;"); !reflect.DeepEqual(got, before) {
		t.Fatal("failed save changed live state")
	}
	if _, err := db.Execute("SELECT * FROM broken;"); err == nil {
		t.Fatal("failed save created table")
	}
	reopened, err := OpenSnapshot(prior)
	if err != nil {
		t.Fatal(err)
	}
	if got := execute(t, reopened, "SELECT * FROM t;"); !reflect.DeepEqual(got, before) {
		t.Fatal("last committed snapshot changed")
	}
	if files, err := filepath.Glob(filepath.Join(dir, ".tacdb-snapshot-*")); err != nil || len(files) != 0 {
		t.Fatalf("temporary snapshots left: %v", files)
	}
}
