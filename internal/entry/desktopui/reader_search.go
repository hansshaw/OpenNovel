package desktopui

import (
	"encoding/hex"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const readerSearchPageSize = 100

type readerSearchMatch struct {
	Chapter     int    `json:"chapter"`
	Title       string `json:"title"`
	Excerpt     string `json:"excerpt"`
	Start       int    `json:"start"`
	Length      int    `json:"length"`
	ContentHash string `json:"contentHash"`
}

// A cursor identifies the first undisplayed match in the lowercased UTF-8
// text. Bind it to the source revision before using it to address that text.
type readerSearchCursor struct {
	chapter    int
	byteOffset int
	hash       string
}

func parseReaderSearchCursor(raw string) (readerSearchCursor, bool) {
	if raw == "" {
		return readerSearchCursor{}, true
	}
	parts := strings.SplitN(raw, ".", 3)
	if len(parts) != 3 || len(parts[2]) != 64 {
		return readerSearchCursor{}, false
	}
	chapter, e1 := strconv.Atoi(parts[0])
	offset, e2 := strconv.Atoi(parts[1])
	_, e3 := hex.DecodeString(parts[2])
	return readerSearchCursor{chapter, offset, parts[2]}, e1 == nil && e2 == nil && e3 == nil && chapter > 0 && offset >= 0
}

func (s *Server) readerSearch(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	query := strings.ToLower(strings.TrimSpace(params.Get("q")))
	length := utf8.RuneCountInString(query)
	if length < 1 || length > 100 || !utf8.ValidString(params.Get("q")) {
		fail(w, 400, "请输入 1–100 字的搜索内容")
		return
	}
	cursor, valid := parseReaderSearchCursor(params.Get("cursor"))
	if !valid {
		fail(w, 400, "搜索位置无效，请重新搜索")
		return
	}
	s.mu.Lock()
	book, ok := s.readerRequestedBook(w, r)
	if !ok {
		s.mu.Unlock()
		return
	}
	// Pin the directory and chapter list, then release the global server
	// lock before scanning a long manuscript. Each result has its own hash.
	root, err := os.OpenRoot(book.Directory)
	if err != nil {
		s.readerError(w, err)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	defer root.Close()
	var scope int
	if raw := params.Get("chapter"); raw != "" {
		scope, err = strconv.Atoi(raw)
		if err != nil || scope < 1 || !readerHasChapter(book, scope) {
			fail(w, 404, "搜索范围中的章节不存在")
			return
		}
	}
	if cursor.chapter > 0 && (!readerHasChapter(book, cursor.chapter) || (scope != 0 && scope != cursor.chapter)) {
		fail(w, 409, "目录已变化，请重新搜索")
		return
	}
	matches := make([]readerSearchMatch, 0, readerSearchPageSize)
	for _, chapter := range book.Chapters {
		if r.Context().Err() != nil {
			return
		}
		if chapter.Number < cursor.chapter || (scope != 0 && chapter.Number != scope) {
			continue
		}
		content, err := readReaderChapterAt(root, chapter.Number)
		if err != nil {
			s.mu.Lock()
			s.readerError(w, err)
			s.mu.Unlock()
			return
		}
		folded := strings.ToLower(content)
		offset, position := 0, 0
		hash := ""
		if chapter.Number == cursor.chapter {
			hash = readerContentHash(content)
			offset = cursor.byteOffset
			if hash != cursor.hash || offset > len(folded) || !strings.HasPrefix(folded[offset:], query) {
				fail(w, 409, "搜索位置已变化，请重新搜索")
				return
			}
			position = utf8.RuneCountInString(folded[:offset])
		}
		var runes []rune
		for offset < len(folded) {
			delta := strings.Index(folded[offset:], query)
			if delta < 0 {
				break
			}
			hit := offset + delta
			if hash == "" {
				hash = readerContentHash(content)
			}
			if len(matches) == readerSearchPageSize {
				next := strconv.Itoa(chapter.Number) + "." + strconv.Itoa(hit) + "." + hash
				reply(w, 200, map[string]any{"matches": matches, "truncated": true, "nextCursor": next})
				return
			}
			if runes == nil {
				runes = []rune(content)
			}
			position += utf8.RuneCountInString(folded[offset:hit])
			start, end := max(0, position-45), min(len(runes), position+length+85)
			excerpt := string(runes[start:end])
			if start > 0 {
				excerpt = "…" + excerpt
			}
			if end < len(runes) {
				excerpt += "…"
			}
			matches = append(matches, readerSearchMatch{chapter.Number, chapter.Title, excerpt, position, length, hash})
			position += length
			offset = hit + len(query)
		}
	}
	reply(w, 200, map[string]any{"matches": matches, "truncated": false})
}
