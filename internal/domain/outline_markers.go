package domain

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeOutlineMarkers is shared by author and model edits. Names are short,
// single-line identifiers whose original case is significant.
func NormalizeOutlineMarkers(names []string) ([]string, error) {
	if len(names) > 32 {
		return nil, fmt.Errorf("每章铺垫和兑现分别最多 32 项")
	}
	var normalized []string
	seen := make(map[string]bool, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 || strings.IndexFunc(name, unicode.IsControl) >= 0 || strings.ContainsAny(name, "\u2028\u2029") {
			return nil, fmt.Errorf("伏笔名称须为 1 到 128 个字符的单行文本")
		}
		if !seen[name] {
			normalized = append(normalized, name)
			seen[name] = true
		}
	}
	return normalized, nil
}
