package desktopui

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/hansshaw/OpenNovel/internal/domain"
	"github.com/hansshaw/OpenNovel/internal/store"
)

func seedChapterEditor(t *testing.T, s *Server) *store.Store {
	t.Helper()
	st := store.NewStore(s.dir)
	for _, err := range []error{
		st.Init(), st.Progress.Init(2), st.Progress.UpdatePhase(domain.PhaseWriting),
		st.RunMeta.Init("default", "local", "test"),
		st.Outline.SavePremise("# 手工编辑测试\n\n第一阶段原则：不改变主角身份。"),
		st.Outline.SaveLayeredOutline([]domain.VolumeOutline{{Index: 1, Title: "初见", Arcs: []domain.ArcOutline{{Index: 1, Title: "探访", Chapters: []domain.OutlineEntry{
			{Title: "旧信", CoreEvent: "收到信件", Hook: "来者何人", Scenes: []string{"站台"}},
			{Title: "来客", CoreEvent: "查访朋友", Hook: "信封被打开", Scenes: []string{"街角"}},
		}}}}}),
		st.Drafts.SaveFinalChapter(1, "原始正文。"),
		st.Summaries.SaveSummary(domain.ChapterSummary{Chapter: 1, Title: "旧信", Summary: "收到旧信"}),
		st.Progress.MarkChapterComplete(1, domain.WordCount("原始正文。"), "", ""),
		st.RunMeta.UpdateWorkbench(func(w *domain.WorkbenchControl) error { w.AcceptedChapter = 1; return nil }),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.ChapterRecords.Accept(1, domain.ChapterOriginGenerated, "原始正文。", domain.ChapterFacts{Title: "旧信", Summary: "收到旧信"}, domain.StyleDelta{}); err != nil {
		t.Fatal(err)
	}
	return st
}

func getChapterEditor(t *testing.T, s *Server, chapter string) chapterEditView {
	t.Helper()
	w := request(s, "GET", "/api/chapters/"+chapter+"/edit?directory="+url.QueryEscape(s.dir), "")
	var view chapterEditView
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &view) != nil {
		t.Fatalf("edit GET: %d %s", w.Code, w.Body.String())
	}
	return view
}

func saveChapterEditor(s *Server, chapter string, body map[string]any) (int, string) {
	encoded, _ := json.Marshal(body)
	w := request(s, "POST", "/api/chapters/"+chapter+"/edit", string(encoded))
	return w.Code, w.Body.String()
}

func TestChapterEditorSavesActualPlanningAndRejectsStaleEdits(t *testing.T) {
	s := newTestServer(t, false)
	st := seedChapterEditor(t, s)
	before := getChapterEditor(t, s, "1")
	other := getChapterEditor(t, s, "2")
	code, body := saveChapterEditor(s, "1", map[string]any{
		"directory": s.dir, "expectedRevision": before.Revision, "savePlan": true,
		"title": "新的章名", "volumeTitle": "新的卷名", "coreEvent": "找回失散信件", "hook": "", "scenes": []string{"旧邮局", "深夜街角"},
	})
	if code != 200 {
		t.Fatalf("save plan: %d %s", code, body)
	}
	after := getChapterEditor(t, s, "1")
	if after.Title != "新的章名" || after.VolumeTitle != "新的卷名" || after.CoreEvent != "找回失散信件" || after.Hook != "" || len(after.Scenes) != 2 || after.Content != before.Content || after.Revision == before.Revision {
		t.Fatalf("saved edit: %+v", after)
	}
	plan, err := store.NewStore(s.dir).Outline.GetChapterOutline(1)
	if err != nil || plan.Title != after.Title || plan.CoreEvent != after.CoreEvent || plan.VolumeTitle != after.VolumeTitle {
		t.Fatalf("generation plan: %+v %v", plan, err)
	}
	if getChapterEditor(t, s, "2").VolumeTitle != "新的卷名" {
		t.Fatal("parent volume rename was not shared by its chapters")
	}
	meta, _ := st.RunMeta.Load()
	if !meta.Workbench.PlanningRevisionPending || meta.Workbench.ApprovedFingerprint != "" {
		t.Fatalf("manual changes must be reaudited: %+v", meta.Workbench)
	}
	if content, _ := st.Drafts.LoadChapterText(1); content != before.Content {
		t.Fatal("plan edit rewrote prose")
	}
	code, _ = saveChapterEditor(s, "2", map[string]any{"directory": s.dir, "expectedRevision": other.Revision, "savePlan": true, "title": "迟到的修改"})
	if code != 409 {
		t.Fatalf("stale parent plan must conflict: %d", code)
	}
}

func TestChapterEditorSavesBodyOfflineAsANewVersion(t *testing.T) {
	s := newTestServer(t, false)
	st := seedChapterEditor(t, s)
	before := getChapterEditor(t, s, "1")
	code, body := saveChapterEditor(s, "1", map[string]any{"directory": s.dir, "expectedRevision": before.Revision, "saveContent": true, "content": "用户亲自修改后的正文。"})
	if code != 200 {
		t.Fatalf("offline body save: %d %s", code, body)
	}
	after := getChapterEditor(t, s, "1")
	if after.Content != "用户亲自修改后的正文。" || !after.SyncRequired {
		t.Fatalf("saved body must require fact sync: %+v", after)
	}
	record, _ := st.ChapterRecords.Load(1)
	if record.Content != before.Content {
		t.Fatal("unsynchronized edits must not fabricate new chapter facts")
	}
	w := request(s, "GET", "/api/chapters/1/versions?directory="+url.QueryEscape(s.dir), "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "原始正文。") || !strings.Contains(w.Body.String(), "用户亲自修改后的正文。") {
		t.Fatalf("original and manual versions: %d %s", w.Code, w.Body.String())
	}
	code, _ = saveChapterEditor(s, "1", map[string]any{"directory": s.dir, "expectedRevision": before.Revision, "saveContent": true, "content": "迟到的正文"})
	if code != 409 {
		t.Fatalf("stale body must conflict: %d", code)
	}
}

func TestChapterEditorCannotSaveToAnotherProjectOrWhileBusy(t *testing.T) {
	s := newTestServer(t, false)
	seedChapterEditor(t, s)
	before := getChapterEditor(t, s, "1")
	code, _ := saveChapterEditor(s, "1", map[string]any{"directory": s.dir + "-other", "expectedRevision": before.Revision, "saveContent": true, "content": "错误项目正文"})
	if code != 409 {
		t.Fatalf("directory guard: %d", code)
	}
	s.mu.Lock()
	s.busy = true
	s.mu.Unlock()
	code, _ = saveChapterEditor(s, "1", map[string]any{"directory": s.dir, "expectedRevision": before.Revision, "saveContent": true, "content": "运行中的改动"})
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
	if code != 409 {
		t.Fatalf("busy guard: %d", code)
	}
}

func TestChapterEditorThreadMarkersRoundTripViaPlanningReview(t *testing.T) {
	s := newTestServer(t, false)
	seedChapterEditor(t, s)
	edit := func(chapter string, fields map[string]any, expected int) {
		t.Helper()
		view := getChapterEditor(t, s, chapter)
		body := map[string]any{"directory": s.dir, "expectedRevision": view.Revision, "savePlan": true,
			"title": view.Title, "volumeTitle": view.VolumeTitle, "coreEvent": view.CoreEvent, "hook": view.Hook, "scenes": view.Scenes}
		for key, value := range fields {
			body[key] = value
		}
		if code, message := saveChapterEditor(s, chapter, body); code != expected {
			t.Fatalf("save markers: %d %s", code, message)
		}
	}
	const thread = "缺角的车票"
	edit("1", map[string]any{"setups": []string{" " + thread, thread}}, 200)
	edit("2", map[string]any{"payoffs": []string{thread}}, 200)
	edit("1", map[string]any{"hook": "看清票根的日期"}, 200) // Older clients omit marker fields.
	assertThread := func(want string) {
		t.Helper()
		var result struct {
			OutlineThreads []struct {
				Name, Status    string
				Setups, Payoffs []int
			}
		}
		w := request(s, "GET", "/api/planning?directory="+url.QueryEscape(s.dir), "")
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.OutlineThreads) != 1 {
			t.Fatal(w.Body.String())
		}
		item := result.OutlineThreads[0]
		if item.Name != thread || item.Status != want || len(item.Setups) != 1 || item.Setups[0] != 1 ||
			(want == "paired" && (len(item.Payoffs) != 1 || item.Payoffs[0] != 2)) {
			t.Fatalf("thread: %+v", item)
		}
	}
	assertThread("paired")
	edit("1", map[string]any{"setups": []string{"invalid\nmarker"}}, 400)
	assertThread("paired")
	edit("2", map[string]any{"payoffs": []string{}}, 200)
	assertThread("missing_payoff")
}
