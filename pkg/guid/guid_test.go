package guid

import (
	"encoding/json"
	"fmt"
	"testing"
)

func Test_New(t *testing.T) {
	g, err := NewV4()
	if err != nil {
		t.Fatal(err)
	}
	g2, err := NewV4()
	if err != nil {
		t.Fatal(err)
	}
	if *g == *g2 {
		t.Fatalf("GUIDs are equal: %s, %s", g, g2)
	}
}

func Test_FromString(t *testing.T) {
	orig := "8e35239e-2084-490e-a3db-ab18ee0744cb"
	g, err := FromString(orig)
	if err != nil {
		t.Fatal(err)
	}
	s := g.String()
	if orig != s {
		t.Fatalf("GUIDs not equal: %s, %s", orig, s)
	}
}

func Test_MarshalJSON(t *testing.T) {
	g, err := NewV4()
	if err != nil {
		t.Fatal(err)
	}
	j, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	gj := fmt.Sprintf("\"%s\"", g.String())
	if string(j) != gj {
		t.Fatalf("JSON not equal: %s, %s", j, gj)
	}
}

func Test_MarshalJSON_Nested(t *testing.T) {
	type test struct {
		G *GUID
	}
	g, err := NewV4()
	if err != nil {
		t.Fatal(err)
	}
	t1 := test{g}
	j, err := json.Marshal(t1)
	if err != nil {
		t.Fatal(err)
	}
	gj := fmt.Sprintf("{\"G\":\"%s\"}", g.String())
	if string(j) != gj {
		t.Fatalf("JSON not equal: %s, %s", j, gj)
	}
}

func Test_UnmarshalJSON(t *testing.T) {
	g, err := NewV4()
	if err != nil {
		t.Fatal(err)
	}
	j, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var g2 GUID
	if err := json.Unmarshal(j, &g2); err != nil {
		t.Fatal(err)
	}
	if *g != g2 {
		t.Fatalf("GUIDs not equal: %s, %s", g, &g2)
	}
}

func Test_UnmarshalJSON_Nested(t *testing.T) {
	type test struct {
		G *GUID
	}
	g, err := NewV4()
	if err != nil {
		t.Fatal(err)
	}
	t1 := test{g}
	j, err := json.Marshal(t1)
	if err != nil {
		t.Fatal(err)
	}
	var t2 test
	if err := json.Unmarshal(j, &t2); err != nil {
		t.Fatal(err)
	}
	if *t1.G != *t2.G {
		t.Fatalf("GUIDs not equal: %v, %v", t1.G, t2.G)
	}
}
