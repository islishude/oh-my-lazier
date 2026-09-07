package txmgr

import (
	"testing"

	"github.com/islishude/oh-my-lazier/go/internal/rpcquorum"
)

func TestVisibilityVerdict(t *testing.T) {
	for _, tc := range []struct {
		name            string
		states          []string
		absent, visible bool
	}{
		{"empty", nil, false, false}, {"single absent", []string{"absent"}, true, false},
		{"majority absent", []string{"absent", "absent", "unavailable"}, true, false},
		{"visible minority veto", []string{"absent", "absent", "pending"}, false, true},
		{"mined veto", []string{"absent", "mined"}, false, true},
		{"timeout is not absent", []string{"absent", "unavailable", "unavailable"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := make([]rpcquorum.TransactionVisibility, len(tc.states))
			for i, s := range tc.states {
				rows[i].State = s
			}
			a, v := visibilityVerdict(rows)
			if a != tc.absent || v != tc.visible {
				t.Fatalf("got %v %v", a, v)
			}
		})
	}
}
