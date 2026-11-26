package revision

import (
	"errors"
	"fmt"

	"github.com/hansshaw/OpenNovel/internal/domain"
	"github.com/hansshaw/OpenNovel/internal/store"
)

// ChapterOpening holds the relationships and unresolved clues at the start of a chapter.
type ChapterOpening struct {
	Foreshadow    []domain.ForeshadowEntry
	Relationships []domain.RelationshipEntry
	records       []domain.ChapterRecord // non-nil when replaying a historical prefix
}

// LoadChapterOpening uses live projections for forward writing. Earlier chapters need a replay:
// filtering today's relationships cannot recover a friendship that later ended, or a resolved clue.
func LoadChapterOpening(st *store.Store, progress *domain.Progress, chapter int) (ChapterOpening, error) {
	if progress == nil || progress.LatestCompleted() < chapter {
		clues, clueErr := st.World.LoadActiveForeshadow()
		relations, relErr := st.World.LoadRelationships()
		return ChapterOpening{Foreshadow: clues, Relationships: relations}, errors.Join(clueErr, relErr)
	}
	var prior []int
	for _, completed := range progress.CompletedChapters {
		if completed < chapter {
			prior = append(prior, completed)
		}
	}
	records, err := st.ChapterRecords.LoadCompleted(prior)
	if err != nil {
		return ChapterOpening{}, fmt.Errorf("读取第 %d 章之前的接纳记录: %w", chapter, err)
	}
	_, ledger, relations, _, err := projectWorld(records)
	if err != nil {
		return ChapterOpening{}, err
	}
	var active []domain.ForeshadowEntry
	for _, clue := range ledger {
		if clue.Status != "resolved" {
			active = append(active, clue)
		}
	}
	return ChapterOpening{Foreshadow: active, Relationships: relations, records: records}, nil
}

// SupportingCast uses the same history as the relationships without loading chapter records twice.
func (o ChapterOpening) SupportingCast(st *store.Store, completed []int) ([]domain.CastEntry, error) {
	if o.records == nil {
		return st.BuildCast(completed)
	}
	characters, err := st.Characters.Load()
	if err != nil {
		return nil, err
	}
	return domain.ProjectCast(o.records, characters), nil
}
