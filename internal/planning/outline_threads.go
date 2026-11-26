package planning

import (
	"slices"
	"strings"

	"github.com/hansshaw/OpenNovel/internal/domain"
)

// OutlineThread indexes an explicitly named promise in the plan. These chapter
// numbers express author intent; the committed foreshadow ledger tracks prose.
type OutlineThread struct {
	Name    string `json:"name"`
	Setups  []int  `json:"setups"`
	Payoffs []int  `json:"payoffs"`
	Status  string `json:"status"`
}

const (
	ThreadPaired            = "paired"
	ThreadMissingSetup      = "missing_setup"
	ThreadMissingPayoff     = "missing_payoff"
	ThreadPayoffBeforeSetup = "payoff_before_setup"
	ThreadSetupAfterPayoff  = "setup_after_payoff"
)

// OutlineThreads groups markers in order of first appearance. A chapter may
// both introduce and answer a question; distinct stages can share a name.
func OutlineThreads(outline []domain.OutlineEntry) []OutlineThread {
	threads := []OutlineThread{}
	index := make(map[string]int)
	add := func(names []string, chapter int, setup bool) {
		for _, raw := range names {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			i, ok := index[name]
			if !ok {
				i = len(threads)
				index[name] = i
				threads = append(threads, OutlineThread{Name: name, Setups: []int{}, Payoffs: []int{}})
			}
			chapters := &threads[i].Payoffs
			if setup {
				chapters = &threads[i].Setups
			}
			*chapters = append(*chapters, chapter)
		}
	}
	for _, entry := range outline {
		if entry.Chapter > 0 {
			add(entry.Setups, entry.Chapter, true)
			add(entry.Payoffs, entry.Chapter, false)
		}
	}
	for i := range threads {
		thread := &threads[i]
		slices.Sort(thread.Setups)
		thread.Setups = slices.Compact(thread.Setups)
		slices.Sort(thread.Payoffs)
		thread.Payoffs = slices.Compact(thread.Payoffs)
		switch {
		case len(thread.Setups) == 0:
			thread.Status = ThreadMissingSetup
		case len(thread.Payoffs) == 0:
			thread.Status = ThreadMissingPayoff
		case thread.Payoffs[0] < thread.Setups[0]:
			thread.Status = ThreadPayoffBeforeSetup
		case thread.Setups[len(thread.Setups)-1] > thread.Payoffs[len(thread.Payoffs)-1]:
			thread.Status = ThreadSetupAfterPayoff
		default:
			thread.Status = ThreadPaired
		}
	}
	return threads
}
