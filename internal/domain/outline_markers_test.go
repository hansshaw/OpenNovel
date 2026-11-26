package domain

import (
	"slices"
	"strings"
	"testing"
)

func TestNormalizeOutlineMarkers(t *testing.T) {
	got, err := NormalizeOutlineMarkers([]string{" 船票  ", "船票", "A", "a"})
	if err != nil || !slices.Equal(got, []string{"船票", "A", "a"}) {
		t.Fatalf("normalized names = %q, err = %v", got, err)
	}
	for _, names := range [][]string{{" "}, {"两\n行"}, {"两\u2028行"}, {string([]byte{0xff})}, {strings.Repeat("书", 129)}, make([]string, 33)} {
		if _, err := NormalizeOutlineMarkers(names); err == nil {
			t.Errorf("accepted invalid markers %q", names)
		}
	}
	if got, err := NormalizeOutlineMarkers([]string{}); err != nil || got != nil {
		t.Fatalf("empty input must share the legacy representation: %q, %v", got, err)
	}
}
