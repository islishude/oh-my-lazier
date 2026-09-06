package txretry

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeStore struct {
	Store
	writes int
}

func (s *fakeStore) InspectTx(context.Context, int64) (json.RawMessage, error) {
	return json.RawMessage(`{"replace_requested_at":"registered-time"}`), nil
}
func (s *fakeStore) RequestTxReplacement(context.Context, int64) error { s.writes++; return nil }
func TestInspectNeverMutatesAndReplaceReportsRegistration(t *testing.T) {
	s := &fakeStore{}
	if _, err := Run(t.Context(), s, Options{ID: 1, Action: "inspect"}); err != nil {
		t.Fatal(err)
	}
	if s.writes != 0 {
		t.Fatal("inspect mutated")
	}
	out, err := Run(t.Context(), s, Options{ID: 1, Action: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		RequestStatus string `json:"request_status"`
	}
	if err = json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if s.writes != 1 || v.RequestStatus != "registered" {
		t.Fatalf("unexpected output %s", out)
	}
}
