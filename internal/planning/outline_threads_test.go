package planning

import (
	"reflect"
	"testing"

	"github.com/hansshaw/OpenNovel/internal/domain"
)

func TestOutlineThreadsPairsMarkersAcrossVolumes(t *testing.T) {
	outline := domain.FlattenOutline([]domain.VolumeOutline{
		{Arcs: []domain.ArcOutline{
			{Chapters: []domain.OutlineEntry{{Setups: []string{" 缺角船票 ", "缺角船票", "失约"}, Payoffs: []string{"倒叙", "无来源"}}}},
			{EstimatedChapters: 12, Chapters: []domain.OutlineEntry{{Payoffs: []string{"失约"}}}},
		}},
		{Arcs: []domain.ArcOutline{{Chapters: []domain.OutlineEntry{
			{Setups: []string{"倒叙", "当章", "灯芯"}, Payoffs: []string{"缺角船票", "当章", "灯芯"}},
			{Setups: []string{"灯芯"}},
		}}}},
	})
	want := []OutlineThread{
		{"缺角船票", []int{1}, []int{2}, ThreadPaired},
		{"失约", []int{1}, []int{}, ThreadMissingPayoff},
		{"倒叙", []int{2}, []int{1}, ThreadPayoffBeforeSetup},
		{"无来源", []int{}, []int{1}, ThreadMissingSetup},
		{"当章", []int{2}, []int{2}, ThreadPaired},
		{"灯芯", []int{2, 3}, []int{2}, ThreadSetupAfterPayoff},
	}
	if got := OutlineThreads(outline); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got := OutlineThreads([]domain.OutlineEntry{{Chapter: 1, Title: "旧大纲"}}); len(got) != 0 {
		t.Fatalf("legacy outline acquired synthetic promises: %+v", got)
	}
}
