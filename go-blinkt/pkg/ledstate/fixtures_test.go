package ledstate

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// The Rust port (src/ledstate.rs) reads and rewrites the same fixture, so
// both implementations agree on the shared state file format.
func TestStateFixtureRoundTrips(t *testing.T) {
	data, err := os.ReadFile("../../../tests/fixtures/state_go.json")
	if err != nil {
		t.Fatal(err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	lte := s.Owners["kubesim-lte-7d9f"]
	if lte.TTL != 5030*time.Millisecond || lte.Updated.Nanosecond() != 123456789 {
		t.Errorf("lte entry = %+v", lte)
	}
	if f := s.Frame(); f[6] != (Pixel{B: 255, L: 5}) || f[4] != (Pixel{}) {
		t.Errorf("frame = %+v", f)
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != strings.TrimSpace(string(data)) {
		t.Errorf("re-encoded state differs:\n got %s\nwant %s", out, data)
	}
}
