package course

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModelStatusJSONTag_OmitEmpty(t *testing.T) {
	// Zero status (legacy docs) must not serialize a "status" key.
	b, err := json.Marshal(Model{Id: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"status"`) {
		t.Errorf("zero Status must be omitted, got %s", b)
	}

	// Non-zero status must serialize as "status":"<value>".
	b, err = json.Marshal(Model{Id: "c1", Status: StatusStale})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"status":"stale"`) {
		t.Errorf("Status stale must serialize, got %s", b)
	}
}

func TestModelEffectiveStatus(t *testing.T) {
	cases := []struct {
		status Status
		want   Status
	}{
		{"", StatusActive},
		{StatusStale, StatusStale},
		{StatusInactive, StatusInactive},
	}
	for _, c := range cases {
		m := Model{Status: c.status}
		if got := m.EffectiveStatus(); got != c.want {
			t.Errorf("EffectiveStatus(%q) = %q, want %q", c.status, got, c.want)
		}
	}
}
