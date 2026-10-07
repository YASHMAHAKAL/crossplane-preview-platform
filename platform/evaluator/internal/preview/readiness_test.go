package preview

import (
	"fmt"
	"strings"
	"testing"
)

func TestXRReadyRequiresCurrentHeadAndGeneration(t *testing.T) {
	name := "incident-tracker-pr-12"
	sha := strings.Repeat("a", 40)
	makeXR := func(generation int, head string, synced, ready string, observed int) []byte {
		return []byte(fmt.Sprintf(`{"metadata":{"name":%q,"generation":%d},"spec":{"pr":{"headSHA":%q}},"status":{"conditions":[{"type":"Synced","status":%q,"observedGeneration":%d},{"type":"Ready","status":%q,"observedGeneration":%d}]}}`, name, generation, head, synced, observed, ready, observed))
	}
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"current and ready", makeXR(3, sha, "True", "True", 3), true},
		{"provider still reconciling", makeXR(3, sha, "True", "False", 3), false},
		{"old generation", makeXR(3, sha, "True", "True", 2), false},
		{"old PR head", makeXR(3, strings.Repeat("b", 40), "True", "True", 3), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := xrReady(tc.data, name, sha)
			if err != nil || got != tc.want {
				t.Fatalf("got %t, want %t: %v", got, tc.want, err)
			}
		})
	}
}
