package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hansshaw/OpenNovel/internal/domain"
	"github.com/hansshaw/OpenNovel/internal/store"
)

func TestWriterContextAndSelfReviewRewindToOpeningOfRequestedChapter(t *testing.T) {
	st := store.NewStore(t.TempDir())
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	check(st.Init())
	check(st.Outline.SaveLayeredOutline([]domain.VolumeOutline{
		{Index: 1, Arcs: []domain.ArcOutline{{Index: 1, Chapters: []domain.OutlineEntry{{Title: "遗物", Setups: []string{"旧账册"}}}}}},
		{Index: 2, Arcs: []domain.ArcOutline{
			{Index: 1, Chapters: []domain.OutlineEntry{{Title: "指路"}}},
			{Index: 2, Title: "寻访", Chapters: []domain.OutlineEntry{{Title: "当面对账", Payoffs: []string{"旧账册"}}}},
			{Index: 3, Title: "终局", Chapters: []domain.OutlineEntry{{Title: "启程"}}},
		}},
	}))
	check(st.Progress.Save(&domain.Progress{
		TotalChapters: 60, CurrentChapter: 5, Layered: true, CurrentVolume: 2, CurrentArc: 3,
		CompletedChapters: []int{1, 2, 3, 4}, PendingRewrites: []int{3},
		HookHistory: []string{"一", "二", "三", "四"},
	}))
	record := func(ch int, text string, facts domain.ChapterFacts) {
		t.Helper()
		_, err := st.ChapterRecords.Accept(ch, domain.ChapterOriginLegacy, text, facts, domain.StyleDelta{})
		check(err)
		check(st.Drafts.SaveFinalChapter(ch, text))
	}
	relationship := func(relation string) []domain.RelationshipEntry {
		return []domain.RelationshipEntry{{CharacterA: "阿岚", CharacterB: "老周", Relation: relation}}
	}
	record(1, "他把账册压进箱底。", domain.ChapterFacts{
		Characters: []string{"老周"}, CastIntros: []domain.CastIntro{{Name: "老周", BriefRole: "码头账房"}},
		ForeshadowUpdates:   []domain.ForeshadowUpdate{{ID: "ledger", Action: "plant", Description: "旧账册"}},
		RelationshipChanges: relationship("合作"),
	})
	var recent []string
	for n := 1; n <= 18; n++ {
		recent = append(recent, fmt.Sprintf("临时访客%02d", n))
	}
	record(2, "阿岚回到码头。", domain.ChapterFacts{
		Characters: recent, ForeshadowUpdates: []domain.ForeshadowUpdate{{ID: "ledger", Action: "advance"}},
	})
	record(3, "两人当场决裂。", domain.ChapterFacts{
		RelationshipChanges: relationship("决裂"), ForeshadowUpdates: []domain.ForeshadowUpdate{{ID: "ledger", Action: "resolve"}},
	})
	record(4, "船上的孩子收好了钥匙。", domain.ChapterFacts{Characters: []string{"未来访客"}})
	check(st.Drafts.SaveChapterPlan(domain.ChapterPlan{
		Chapter: 3, Contract: domain.ChapterContract{ContinuityChecks: []string{"老周仍在码头"}},
	}))
	check(st.Drafts.SaveDraft(3, "写给自审的草稿"))
	check(st.World.SaveTimeline([]domain.TimelineEvent{{Chapter: 2, Event: "前往码头"}, {Chapter: 4, Event: "离港"}}))
	// The book-wide projections have already moved past chapter 3.
	check(st.World.SaveRelationships(relationship("决裂")))
	check(st.World.SaveForeshadowLedger([]domain.ForeshadowEntry{{ID: "ledger", Status: "resolved"}}))
	check(st.Characters.SaveSnapshots(2, 1, []domain.CharacterSnapshot{{Name: "阿岚", Status: "刚到码头"}}))
	check(st.Characters.SaveSnapshots(2, 3, []domain.CharacterSnapshot{{Name: "阿岚", Status: "已经离港"}}))
	check(st.Summaries.SaveVolumeSummary(domain.VolumeSummary{Volume: 1, Summary: "初入城"}))
	check(st.Summaries.SaveVolumeSummary(domain.VolumeSummary{Volume: 2, Summary: "码头终局"}))
	check(st.Summaries.SaveArcSummary(domain.ArcSummary{Volume: 2, Arc: 1, Summary: "找到了码头"}))
	check(st.Summaries.SaveArcSummary(domain.ArcSummary{Volume: 2, Arc: 2, Summary: "已对完账"}))

	args := json.RawMessage(`{"chapter":3}`)
	raw, err := newTestContextTool(st, References{}, "default").Execute(context.Background(), args)
	check(err)
	var payload struct {
		Working struct {
			Sources   map[string][]int       `json:"payoff_setup_chapters"`
			Timeline  []domain.TimelineEvent `json:"timeline"`
			Volumes   []domain.VolumeSummary `json:"volume_summaries"`
			Arcs      []domain.ArcSummary    `json:"arc_summaries"`
			Following struct {
				Chapter int
				Excerpt string
			} `json:"following_chapter_opening"`
			Checkpoint struct {
				Hooks []string `json:"hook_history"`
			} `json:"checkpoint"`
		} `json:"working_memory"`
		Episodic struct {
			Foreshadow    []domain.ForeshadowEntry   `json:"foreshadow_ledger"`
			Relationships []domain.RelationshipEntry `json:"relationship_state"`
			Snapshots     []domain.CharacterSnapshot `json:"character_snapshots"`
			Cast          []struct {
				Name string
				Last int `json:"last_seen"`
			} `json:"recent_cast"`
			Position struct {
				Arc   int
				Title string `json:"arc_title"`
			} `json:"position"`
		} `json:"episodic_memory"`
	}
	check(json.Unmarshal(raw, &payload))
	w, e := payload.Working, payload.Episodic
	if !slices.Equal(w.Sources["旧账册"], []int{1}) || len(w.Timeline) != 1 || w.Timeline[0].Chapter != 2 || len(w.Checkpoint.Hooks) != 2 {
		t.Fatalf("wrong opening chapter or plan references: %+v", w)
	}
	if len(w.Arcs) != 1 || w.Arcs[0].Arc != 1 || len(w.Volumes) != 1 || w.Volumes[0].Volume != 1 || e.Position.Arc != 2 || e.Position.Title != "寻访" {
		t.Fatalf("wrong historical arc: %+v, %+v", w, e.Position)
	}
	if len(e.Snapshots) != 1 || e.Snapshots[0].Status != "刚到码头" || len(e.Foreshadow) != 1 || e.Foreshadow[0].Status != "advanced" || len(e.Relationships) != 1 || e.Relationships[0].Relation != "合作" {
		t.Fatalf("story facts must precede chapter 3: %+v", e)
	}
	if len(e.Cast) != 15 || e.Cast[0].Name != "老周" || e.Cast[0].Last != 1 || strings.Contains(string(raw), "未来访客") {
		t.Fatalf("lost returning character or retrieved future cast: %+v", e.Cast)
	}
	if w.Following.Chapter != 4 || w.Following.Excerpt != "船上的孩子收好了钥匙。" {
		t.Fatalf("missing next opening in rewrite brief: %+v", w.Following)
	}

	raw, err = NewCheckConsistencyTool(st).Execute(context.Background(), args)
	check(err)
	var review struct {
		Plan          domain.ChapterPlan         `json:"chapter_plan"`
		Foreshadow    []domain.ForeshadowEntry   `json:"foreshadow_ledger"`
		Relationships []domain.RelationshipEntry `json:"relationships"`
		Sources       map[string][]int           `json:"payoff_setup_chapters"`
	}
	check(json.Unmarshal(raw, &review))
	if !slices.Equal(review.Sources["旧账册"], w.Sources["旧账册"]) || review.Plan.Chapter != 3 || !slices.Equal(review.Foreshadow, e.Foreshadow) || !slices.Equal(review.Relationships, e.Relationships) {
		t.Fatalf("writer and self-review disagree: %+v", review)
	}
}
