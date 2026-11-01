package render

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/TomiCode/cnsl/internal/tmpl"
)

// Block is one piece of a region edited without HTML: a component call with arguments, or a raw
// HTML/template snippet. The region's arrangement places the blocks; the template is generated.
type Block struct {
	Component string              `json:"component,omitempty"`
	Args      map[string]BlockArg `json:"args,omitempty"`
	HTML      string              `json:"html,omitempty"` // used when Component is ""
}

// BlockArg is an input value: a data path (.Data.ha.sensor_x) or a literal.
type BlockArg struct {
	Path  string `json:"path,omitempty"`
	Value any    `json:"value,omitempty"`
}

func ParseBlocks(s string) ([]Block, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var blocks []Block
	if err := json.Unmarshal([]byte(s), &blocks); err != nil {
		return nil, fmt.Errorf("invalid blocks: %w", err)
	}
	return blocks, nil
}

// BlocksJSON encodes blocks for storage ("" for HTML mode).
func BlocksJSON(blocks []Block) string {
	if blocks == nil {
		return ""
	}
	b, _ := json.Marshal(blocks)
	return string(b)
}

var dataPath = regexp.MustCompile(`^(\.[A-Za-z_][A-Za-z0-9_]*)+$`)

// BlocksTemplate generates a region template from blocks. Each block is wrapped in a
// display:contents element that marks it for the editor (clicking the preview finds the block)
// without changing the layout.
func BlocksTemplate(blocks []Block, lib tmpl.Library) (string, error) {
	var b strings.Builder
	for i, bl := range blocks {
		fmt.Fprintf(&b, `<div class="cnsl-block" data-cnsl-block="%d">`, i)
		if bl.Component == "" {
			b.WriteString(bl.HTML)
		} else {
			call, err := componentCall(bl, lib)
			if err != nil {
				return "", fmt.Errorf("block %d: %w", i+1, err)
			}
			b.WriteString(call)
		}
		b.WriteString("</div>\n")
	}
	return b.String(), nil
}

func componentCall(bl Block, lib tmpl.Library) (string, error) {
	comp, ok := lib.Component(bl.Component)
	if !ok {
		return "", fmt.Errorf("unknown component %q", bl.Component)
	}
	known := map[string]bool{}
	for _, in := range comp.Inputs {
		known[in.Name] = true
	}
	for name := range bl.Args {
		if !known[name] {
			return "", fmt.Errorf("%s has no input %q", bl.Component, name)
		}
	}
	var b strings.Builder
	b.WriteString("{{ component " + strconv.Quote(comp.Key))
	for _, in := range comp.Inputs { // inputs in their declared order
		arg, ok := bl.Args[in.Name]
		if !ok {
			continue
		}
		expr, err := argExpr(in, arg)
		if err != nil {
			return "", fmt.Errorf("%s: %w", in.Name, err)
		}
		if expr != "" {
			b.WriteString(" " + strconv.Quote(in.Name) + " " + expr)
		}
	}
	b.WriteString(" }}")
	return b.String(), nil
}

// argExpr is the template expression for an argument ("" = leave the input at its default).
func argExpr(in tmpl.Input, arg BlockArg) (string, error) {
	if arg.Path != "" {
		if !dataPath.MatchString(arg.Path) {
			return "", fmt.Errorf("%q is not a data path like .Data.alias.field", arg.Path)
		}
		return arg.Path, nil
	}
	switch v := arg.Value.(type) {
	case nil:
		return "", nil
	case string:
		if v == "" {
			return "", nil // an empty field keeps the default
		}
		if in.Kind == "number" {
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				return "", fmt.Errorf("%q is not a number", v)
			}
			return v, nil
		}
		return strconv.Quote(v), nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(v), nil
	}
	return "", fmt.Errorf("unsupported value %v", arg.Value)
}
