package guid

import (
	"encoding/json"
	"fmt"
	"testing"
)

func mustNewV4(t *testing.T) GUID {
	g, err := NewV4()
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func mustFromString(t *testing.T, s string) GUID {
	g, err := FromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func Test_NewV4IsUnique(t *testing.T) {
	g := mustNewV4(t)
	g2 := mustNewV4(t)
	if g == g2 {
		t.Fatalf("GUIDs are equal: %s, %s", g, g2)
	}
}

func Test_V4HasCorrectVersionAndVariant(t *testing.T) {
	g := mustNewV4(t)
	if g.Version() != 4 {
		t.Fatalf("Version is not 4: %s", g)
	}
	if g.Variant() != VariantRFC4122 {
		t.Fatalf("Variant is not RFC4122: %s", g)
	}
}

func Test_ToArray(t *testing.T) {
	g := mustFromString(t, "73c39589-192e-4c64-9acf-6c5d0aa18528")
	b := g.ToArray()
	expected := [16]byte{0x73, 0xc3, 0x95, 0x89, 0x19, 0x2e, 0x4c, 0x64, 0x9a, 0xcf, 0x6c, 0x5d, 0x0a, 0xa1, 0x85, 0x28}
	if b != expected {
		t.Fatalf("GUID does not match array form: %x, %x", expected, b)
	}
}

func Test_FromArrayAndBack(t *testing.T) {
	b := [16]byte{0x73, 0xc3, 0x95, 0x89, 0x19, 0x2e, 0x4c, 0x64, 0x9a, 0xcf, 0x6c, 0x5d, 0x0a, 0xa1, 0x85, 0x28}
	b2 := FromArray(b).ToArray()
	if b != b2 {
		t.Fatalf("Arrays do not match: %x, %x", b, b2)
	}
}

func Test_ToWindowsArray(t *testing.T) {
	g := mustFromString(t, "73c39589-192e-4c64-9acf-6c5d0aa18528")
	b := g.ToWindowsArray()
	expected := [16]byte{0x89, 0x95, 0xc3, 0x73, 0x2e, 0x19, 0x64, 0x4c, 0x9a, 0xcf, 0x6c, 0x5d, 0x0a, 0xa1, 0x85, 0x28}
	if b != expected {
		t.Fatalf("GUID does not match array form: %x, %x", expected, b)
	}
}

func Test_FromWindowsArrayAndBack(t *testing.T) {
	b := [16]byte{0x73, 0xc3, 0x95, 0x89, 0x19, 0x2e, 0x4c, 0x64, 0x9a, 0xcf, 0x6c, 0x5d, 0x0a, 0xa1, 0x85, 0x28}
	b2 := FromWindowsArray(b).ToWindowsArray()
	if b != b2 {
		t.Fatalf("Arrays do not match: %x, %x", b, b2)
	}
}

func Test_FromString(t *testing.T) {
	orig := "8e35239e-2084-490e-a3db-ab18ee0744cb"
	g := mustFromString(t, orig)
	s := g.String()
	if orig != s {
		t.Fatalf("GUIDs not equal: %s, %s", orig, s)
	}
}

func Test_MarshalJSON(t *testing.T) {
	g := mustNewV4(t)
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
		G GUID
	}
	g := mustNewV4(t)
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
	g := mustNewV4(t)
	j, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var g2 GUID
	if err := json.Unmarshal(j, &g2); err != nil {
		t.Fatal(err)
	}
	if g != g2 {
		t.Fatalf("GUIDs not equal: %s, %s", g, g2)
	}
}

func Test_UnmarshalJSON_Nested(t *testing.T) {
	type test struct {
		G GUID
	}
	g := mustNewV4(t)
	t1 := test{g}
	j, err := json.Marshal(t1)
	if err != nil {
		t.Fatal(err)
	}
	var t2 test
	if err := json.Unmarshal(j, &t2); err != nil {
		t.Fatal(err)
	}
	if t1.G != t2.G {
		t.Fatalf("GUIDs not equal: %v, %v", t1.G, t2.G)
	}
}
