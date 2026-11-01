package render

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
)

// Arrange is how a region lays out its content, so templates (and blocks) only list the pieces:
// the region supplies the framework's single .layout and puts the pieces in a stack, a row, a grid
// or flowing columns. Mode "" means the template brings its own layout (raw HTML).
type Arrange struct {
	Mode  string `json:"mode"`            // "", stack, row, grid, columns
	Cols  int    `json:"cols,omitempty"`  // grid: 1–6 columns, 0 = as many as fit
	Align string `json:"align,omitempty"` // top, center, bottom
	Gap   string `json:"gap,omitempty"`   // none, small, medium, large, xlarge
	// TitleBar is a template for the framework title bar below the layout ("" = none).
	TitleBar string `json:"title_bar,omitempty"`
}

// DefaultArrange is what new regions get.
var DefaultArrange = Arrange{Mode: "stack", Align: "center", Gap: "medium", TitleBar: "{{ .Screen.Name }}"}

func ParseArrange(s string) (Arrange, error) {
	var a Arrange
	if strings.TrimSpace(s) == "" {
		return a, nil
	}
	if err := json.Unmarshal([]byte(s), &a); err != nil {
		return a, fmt.Errorf("invalid arrangement: %w", err)
	}
	return a, nil
}

func (a Arrange) JSON() string {
	if a.Mode == "" {
		return ""
	}
	b, _ := json.Marshal(a)
	return string(b)
}

func (a Arrange) Validate() error {
	switch a.Mode {
	case "", "stack", "row", "grid", "columns":
	default:
		return fmt.Errorf("unknown arrangement %q", a.Mode)
	}
	switch a.Align {
	case "", "top", "center", "bottom":
	default:
		return fmt.Errorf("align must be top, center or bottom, not %q", a.Align)
	}
	switch a.Gap {
	case "", "none", "small", "medium", "large", "xlarge":
	default:
		return fmt.Errorf("unknown gap %q", a.Gap)
	}
	if a.Cols < 0 || a.Cols > 6 {
		return fmt.Errorf("grid columns must be 1–6 (or 0 for automatic)")
	}
	return nil
}

func (a Arrange) align() string {
	if a.Align == "" {
		return "center"
	}
	return a.Align
}

func (a Arrange) gap() string {
	if a.Gap == "" {
		return "gap--medium"
	}
	return "gap--" + a.Gap
}

// cols is the grid's column class, or the marker for the page script that picks a count from the
// number of items and the region's shape.
func (a Arrange) cols() string {
	if a.Cols > 0 {
		return fmt.Sprintf(` class="grid grid--cols-%d %s`, a.Cols, a.gap())
	}
	return fmt.Sprintf(` data-cnsl-grid="auto" class="grid %s`, a.gap())
}

// gapVar is the framework spacing for the flow layout, which can't use the gap utilities.
func (a Arrange) gapVar() string {
	if a.Gap == "none" {
		return "0px"
	}
	return fmt.Sprintf("var(--gap-%s)", strings.TrimPrefix(a.gap(), "gap--"))
}

// wrap puts a region's rendered content into its arrangement. free regions have no framework view
// around them, so they get plain flex/grid containers filling the region instead of .layout.
func (a Arrange) wrap(body template.HTML, free bool) string {
	if free {
		fill := ` style="width:100%;height:100%"`
		flexAlign := map[string]string{"top": "flex--top", "center": "flex--center-y", "bottom": "flex--bottom"}[a.align()]
		switch a.Mode {
		case "row":
			return fmt.Sprintf(`<div class="flex flex--row flex--center-y %s"%s>%s</div>`, a.gap(), fill, body)
		case "grid":
			return fmt.Sprintf(`<div%s" style="width:100%%">%s</div>`, a.cols(), body)
		default: // stack (columns needs a view; stack is the closest)
			return fmt.Sprintf(`<div class="flex flex--col %s %s"%s>%s</div>`, flexAlign, a.gap(), fill, body)
		}
	}
	layoutAlign := "layout--" + a.align()
	switch a.Mode {
	case "row":
		return fmt.Sprintf(`<div class="layout layout--row %s %s">%s</div>`, layoutAlign, a.gap(), body)
	case "grid":
		return fmt.Sprintf(`<div class="layout layout--col %s"><div%s w--full">%s</div></div>`, layoutAlign, a.cols(), body)
	case "columns":
		// Items flow top to bottom into as many columns as they need (see the page script).
		return fmt.Sprintf(`<div class="layout layout--col layout--top"><div class="cnsl-flow" data-cnsl-flow style="--cnsl-gap:%s">%s</div></div>`, a.gapVar(), body)
	default:
		return fmt.Sprintf(`<div class="layout layout--col %s %s">%s</div>`, layoutAlign, a.gap(), body)
	}
}
