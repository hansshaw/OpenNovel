(() => {
  "use strict";

  function compareParagraphs(original, candidate) {
    const paragraphs = text => (text || "").split(/\r\n|[\r\n]/).filter(line => line.trim());
    const before = paragraphs(original), after = paragraphs(candidate);
    let start = 0, beforeEnd = before.length, afterEnd = after.length;
    while (start < beforeEnd && start < afterEnd && before[start] === after[start]) start++;
    while (beforeEnd > start && afterEnd > start && before[beforeEnd - 1] === after[afterEnd - 1]) {
      beforeEnd--;
      afterEnd--;
    }
    const changes = [];
    const rows = beforeEnd - start, cols = afterEnd - start;
    const grouped = rows > 0 && cols > 0 && (rows + 1) * (cols + 1) > 500000;
    // Bound the LCS table in the renderer. Large replacements can still be
    // inspected in full as one range between the unchanged prefix and suffix.
    if (!rows || !cols || grouped) {
      if (rows || cols) changes.push({ from: start, to: beforeEnd, otherFrom: start, otherTo: afterEnd });
      return { before, after, changes, grouped };
    }

    const width = cols + 1;
    const lengths = new Uint16Array((rows + 1) * width);
    for (let row = rows - 1; row >= 0; row--) {
      for (let col = cols - 1; col >= 0; col--) {
        const index = row * width + col;
        lengths[index] = before[start + row] === after[start + col]
          ? lengths[index + width + 1] + 1 : Math.max(lengths[index + width], lengths[index + 1]);
      }
    }
    let row = 0, col = 0, from = start, otherFrom = start;
    function appendChange() {
      const to = start + row, otherTo = start + col;
      if (from < to || otherFrom < otherTo) changes.push({ from, to, otherFrom, otherTo });
    }
    while (row < rows || col < cols) {
      if (row < rows && col < cols && before[start + row] === after[start + col]) {
        appendChange();
        row++;
        col++;
        from = start + row;
        otherFrom = start + col;
      } else if (col === cols || (row < rows && lengths[(row + 1) * width + col] >= lengths[row * width + col + 1])) row++;
      else col++;
    }
    appendChange();
    return { before, after, changes, grouped };
  }

  window.NovelChapterComparison = {
    create() {
      const $ = id => document.getElementById(id);
      const dialog = $("chapter-compare-dialog");
      let preview = null, position = 0;
      function render() {
        const { before, after, changes, grouped, number } = preview;
        const part = changes[position];
        $("previous-chapter-change").disabled = position === 0;
        $("next-chapter-change").disabled = position + 1 >= changes.length;
        $("chapter-compare-panels").classList.toggle("hidden", !part);
        $("chapter-compare-panels").scrollTop = 0;
        if (!part) {
          $("chapter-compare-position").textContent = "两版正文的非空段落一致。";
          return;
        }
        const kind = part.from === part.to ? "新增" : part.otherFrom === part.otherTo ? "删除" : "修改";
        $("chapter-compare-position").textContent = grouped ? "改动范围较长，已合并展示首尾之间的段落。" : `第 ${position + 1} / ${changes.length} 处 · ${kind}`;
        function show(id, title, lines, from, to) {
          const range = to === from ? "" : to === from + 1 ? ` · 第 ${to} 段` : ` · 第 ${from + 1}–${to} 段`;
          $(id + "-label").textContent = title + range;
          $(id).textContent = lines.slice(from, to).join("\n\n") || "此处没有对应段落";
        }
        show("chapter-compare-before", "当前正式版", before, part.from, part.to);
        show("chapter-compare-after", `所选版本 ${number}`, after, part.otherFrom, part.otherTo);
      }
      $("previous-chapter-change").addEventListener("click", () => { if (preview && position > 0) { position--; render(); } });
      $("next-chapter-change").addEventListener("click", () => { if (preview && position + 1 < preview.changes.length) { position++; render(); } });
      $("close-chapter-compare").addEventListener("click", () => dialog.close());
      dialog.addEventListener("close", () => {
        if (dialog.open) return;
        preview = null;
        $("chapter-compare-before").textContent = $("chapter-compare-after").textContent = "";
      });
      return {
        open(chapter, original, version) {
          preview = { ...compareParagraphs(original.content, version.content), number: version.number };
          position = 0;
          $("chapter-compare-title").textContent = `第 ${chapter} 章 · 正式版与版本 ${version.number} 对照`;
          render();
          dialog.showModal();
        },
        close() { if (dialog.open) dialog.close(); },
      };
    },
  };
})();
