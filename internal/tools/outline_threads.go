package tools

import (
	"slices"

	"github.com/hansshaw/OpenNovel/internal/domain"
	"github.com/hansshaw/OpenNovel/internal/planning"
)

type outlineThreadIssue struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	SetupChapter  int    `json:"setup_chapter,omitempty"`
	PayoffChapter int    `json:"payoff_chapter,omitempty"`
}

// Add compact evidence even when the planning view omits the relevant arc.
// The desktop review can display the full index without the model token budget.
func addOutlineThreadReview(result map[string]any, outline []domain.OutlineEntry) {
	threads := planning.OutlineThreads(outline)
	if len(threads) == 0 {
		return
	}
	const issueLimit = 12
	issueCount := 0
	issues := []outlineThreadIssue{}
	for _, thread := range threads {
		if thread.Status == planning.ThreadPaired {
			continue
		}
		issueCount++
		if len(issues) >= issueLimit {
			continue
		}
		issue := outlineThreadIssue{Name: thread.Name, Status: thread.Status}
		if len(thread.Setups) > 0 {
			issue.SetupChapter = thread.Setups[0]
		}
		if len(thread.Payoffs) > 0 {
			issue.PayoffChapter = thread.Payoffs[0]
		}
		if thread.Status == planning.ThreadSetupAfterPayoff {
			issue.SetupChapter = thread.Setups[len(thread.Setups)-1]
			issue.PayoffChapter = thread.Payoffs[len(thread.Payoffs)-1]
		}
		issues = append(issues, issue)
	}
	result["outline_thread_review"] = map[string]any{
		"thread_count": len(threads), "issue_count": issueCount, "issues": issues,
	}
}

// Only expose earlier setup references for promises due in the current chapter.
// For a recurrent thread, the initial clue and two most recent reminders suffice.
func priorPayoffSetups(outline []domain.OutlineEntry, chapter int) map[string][]int {
	result := map[string][]int{}
	for _, thread := range planning.OutlineThreads(outline) {
		if _, due := slices.BinarySearch(thread.Payoffs, chapter); !due {
			continue
		}
		end, _ := slices.BinarySearch(thread.Setups, chapter)
		if end == 0 {
			continue
		}
		chapters := thread.Setups[:end]
		if end > 3 {
			chapters = []int{chapters[0], chapters[end-2], chapters[end-1]}
		}
		result[thread.Name] = chapters
	}
	return result
}
