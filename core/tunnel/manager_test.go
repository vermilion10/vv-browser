package tunnel

import (
	"path/filepath"
	"reflect"
	"testing"
)

func ids(cs []Candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func TestPutFirst(t *testing.T) {
	list := []Candidate{{ID: "a"}, {ID: "b", Name: "fresh"}, {ID: "c"}}

	got := putFirst(list, Candidate{ID: "b", Name: "saved"})
	if want := []string{"b", "a", "c"}; !reflect.DeepEqual(ids(got), want) {
		t.Errorf("listed preferred: got %v, want %v", ids(got), want)
	}
	if got[0].Name != "fresh" {
		t.Errorf("listed preferred should use the list's fresher entry, got %q", got[0].Name)
	}

	got = putFirst(list, Candidate{ID: "z"})
	if want := []string{"z", "a", "b", "c"}; !reflect.DeepEqual(ids(got), want) {
		t.Errorf("unlisted preferred: got %v, want %v", ids(got), want)
	}
}

func TestPreferredRoundTrip(t *testing.T) {
	m := &Manager{StatePath: filepath.Join(t.TempDir(), "sub", "relay.json")}
	if _, ok := m.loadPreferred(); ok {
		t.Fatal("expected no preferred server before saving")
	}
	want := Candidate{ID: "vpngate:x", Name: "x", Config: []byte("remote 1.2.3.4 443")}
	m.savePreferred(want)
	got, ok := m.loadPreferred()
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("loadPreferred() = %+v, %v; want %+v", got, ok, want)
	}
	m.ForgetPreferred()
	if _, ok := m.loadPreferred(); ok {
		t.Fatal("expected no preferred server after ForgetPreferred")
	}
}
