package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/hansshaw/OpenNovel/internal/domain"
	"github.com/hansshaw/OpenNovel/internal/errs"
)

func TestOutlineThreadContextCrossesHiddenArcsAndUsesOnlyPastSetups(t *testing.T) {
	st := completeShortFoundation(t)
	var chapters []domain.OutlineEntry
	for i := 1; i <= 7; i++ {
		chapters = append(chapters, domain.OutlineEntry{
			Title: fmt.Sprintf("第 %d 章", i), CoreEvent: "沿海寻找父亲", Hook: "灯亮了", Setups: []string{"灯号"},
		})
	}
	chapters[5].Payoffs = []string{"灯号", "孤单的船票"}
	if _, err := st.ReplaceLayeredOutline([]domain.VolumeOutline{{Title: "海", Arcs: []domain.ArcOutline{
		{Title: "近岸", Goal: "寻找", Chapters: chapters[:1]}, {Title: "深海", Goal: "相会", Chapters: chapters[1:]},
	}}}); err != nil {
		t.Fatal(err)
	}
	tool := newTestContextTool(st, References{}, "default")
	architect, err := tool.Execute(context.Background(), json.RawMessage(`{"volume":1,"arc":1}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Planning struct {
			Review struct {
				IssueCount int                  `json:"issue_count"`
				Issues     []outlineThreadIssue `json:"issues"`
			} `json:"outline_thread_review"`
		} `json:"planning_memory"`
	}
	if err := json.Unmarshal(architect, &result); err != nil {
		t.Fatal(err)
	}
	r := result.Planning.Review
	if r.IssueCount != 2 || len(r.Issues) != 2 ||
		r.Issues[0].SetupChapter != 7 || r.Issues[0].PayoffChapter != 6 ||
		r.Issues[1].Name != "孤单的船票" || r.Issues[1].PayoffChapter != 6 {
		t.Fatalf("missing cross-arc evidence: %+v", r)
	}
	writer, err := tool.Execute(context.Background(), json.RawMessage(`{"chapter":6}`))
	if err != nil {
		t.Fatal(err)
	}
	var work struct {
		Memory struct {
			Outline domain.OutlineEntry `json:"current_chapter_outline"`
			Setups  map[string][]int    `json:"payoff_setup_chapters"`
		} `json:"working_memory"`
	}
	if err := json.Unmarshal(writer, &work); err != nil {
		t.Fatal(err)
	}
	if len(work.Memory.Setups) != 1 || !slices.Equal(work.Memory.Setups["灯号"], []int{1, 4, 5}) ||
		!slices.Equal(work.Memory.Outline.Payoffs, chapters[5].Payoffs) {
		t.Fatalf("writer needs only earlier setup chapters: %+v", work.Memory)
	}
}

func TestRejectedOutlineMarkersPreservePlanAndTier(t *testing.T) {
	st := completeShortFoundation(t)
	if err := st.RunMeta.SetPlanningTier(domain.PlanningTierShort); err != nil {
		t.Fatal(err)
	}
	before, err := st.PlanningFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`{"type":"outline","scale":"long","content":[{"title":"新章","core_event":"抵达","hook":"敲门","setups":[" "]}]}`)
	_, err = NewSaveFoundationTool(st).Execute(context.Background(), args)
	if !errors.Is(err, errs.ErrToolArgs) {
		t.Fatalf("invalid marker error = %v", err)
	}
	after, err := st.PlanningFingerprint()
	if err != nil || before != after {
		t.Fatalf("rejected edit changed plan: %q, %v", after, err)
	}
	meta, err := st.RunMeta.Load()
	if err != nil || meta.PlanningTier != domain.PlanningTierShort {
		t.Fatalf("rejected edit changed planning tier: %+v, %v", meta, err)
	}
}
