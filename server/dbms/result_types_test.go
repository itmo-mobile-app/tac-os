package dbms

import (
	"reflect"
	"testing"
)

func TestResultTypes(t *testing.T) {
	db := sample(t)
	r := execute(t, db, "SELECT name,id,name,score FROM sample;")
	want := []Type{Text, Integer, Text, Real}
	if !reflect.DeepEqual(r.Types, want) || len(r.Rows) != 0 {
		t.Fatalf("empty result metadata: %+v", r)
	}
	r.Types[0] = Integer
	if got := execute(t, db, "SELECT name,id,name,score FROM sample;"); !reflect.DeepEqual(got.Types, want) {
		t.Fatal("result metadata aliases schema")
	}
}
