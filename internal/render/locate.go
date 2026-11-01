package render

import (
	"strconv"
	"strings"
)

// Box is where a piece of a region ended up on the screen (as designed, in device pixels), so the
// editor can map a click on the preview back to the block or template line that produced it.
type Box struct {
	Region string  `json:"region"`
	Block  *int    `json:"block,omitempty"`
	Line   *int    `json:"line,omitempty"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
}

// annotateLines marks an HTML template for locating (previews only): every opening tag gets
// data-cnsl-line with its source line, and every {{ component … }} call is wrapped in a
// display:contents element with its line, since a component's markup has no lines of its own.
// Only insertions within lines are made, so line numbers of errors stay right.
func annotateLines(src string) string {
	var b strings.Builder
	b.Grow(len(src) + len(src)/4)
	line := 1
	for i := 0; i < len(src); {
		switch {
		case strings.HasPrefix(src[i:], "{{"):
			end := strings.Index(src[i:], "}}")
			if end < 0 {
				b.WriteString(src[i:])
				return b.String()
			}
			action := src[i : i+end+2]
			body := strings.TrimSpace(strings.Trim(strings.TrimSpace(action[2:len(action)-2]), "-"))
			if strings.HasPrefix(body, "component ") {
				b.WriteString(`<div class="cnsl-block" data-cnsl-line="` + strconv.Itoa(line) + `">` + action + `</div>`)
			} else {
				b.WriteString(action)
			}
			line += strings.Count(action, "\n")
			i += len(action)
		case strings.HasPrefix(src[i:], "<!--"):
			end := strings.Index(src[i:], "-->")
			if end < 0 {
				b.WriteString(src[i:])
				return b.String()
			}
			chunk := src[i : i+end+3]
			b.WriteString(chunk)
			line += strings.Count(chunk, "\n")
			i += len(chunk)
		case src[i] == '<' && i+1 < len(src) && isTagStart(src[i+1]):
			j := i + 1
			for j < len(src) && isTagChar(src[j]) {
				j++
			}
			name := strings.ToLower(src[i+1 : j])
			b.WriteString(src[i:j])
			if name != "script" && name != "style" {
				b.WriteString(` data-cnsl-line="` + strconv.Itoa(line) + `"`)
			}
			i = j
		default:
			if src[i] == '\n' {
				line++
			}
			b.WriteByte(src[i])
			i++
		}
	}
	return b.String()
}

func isTagStart(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isTagChar(c byte) bool  { return isTagStart(c) || c >= '0' && c <= '9' || c == '-' }

// locateScript collects the boxes of regions, blocks and annotated elements after layout.
// Blocks are display:contents, so their box is the union of their children's boxes.
const locateScript = `(() => {
  const out = [];
  const box = (el) => {
    let r = el.getBoundingClientRect();
    if (getComputedStyle(el).display === 'contents') {
      let x1 = Infinity, y1 = Infinity, x2 = -Infinity, y2 = -Infinity;
      for (const c of el.children) {
        const b = c.getBoundingClientRect();
        if (!b.width && !b.height) continue;
        x1 = Math.min(x1, b.left); y1 = Math.min(y1, b.top); x2 = Math.max(x2, b.right); y2 = Math.max(y2, b.bottom);
      }
      if (x1 === Infinity) return null;
      return { x: x1, y: y1, w: x2 - x1, h: y2 - y1 };
    }
    if (!r.width || !r.height) return null;
    return { x: r.left, y: r.top, w: r.width, h: r.height };
  };
  document.querySelectorAll('[data-region]').forEach((region) => {
    const key = region.dataset.region, rb = box(region);
    if (rb) out.push({ region: key, ...rb });
    region.querySelectorAll('[data-cnsl-block]').forEach((el) => {
      const b = box(el);
      if (b) out.push({ region: key, block: Number(el.dataset.cnslBlock), ...b });
    });
    region.querySelectorAll('[data-cnsl-line]').forEach((el) => {
      const b = box(el);
      if (b) out.push({ region: key, line: Number(el.dataset.cnslLine), ...b });
    });
  });
  return JSON.stringify(out);
})()`
