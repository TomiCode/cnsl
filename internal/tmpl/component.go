package tmpl

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"regexp"
)

// Input declares one parameter of a component.
type Input struct {
	Name      string `json:"name"`                // field name inside the component (.name)
	Label     string `json:"label,omitempty"`     //
	Kind      string `json:"kind"`                // data | string | number | bool
	Connector string `json:"connector,omitempty"` // connector type the data comes from (insert flow, samples)
	Default   any    `json:"default,omitempty"`
	Help      string `json:"help,omitempty"`
	// Sample is example data for previews; without it, data inputs use the connector's sample.
	Sample any `json:"sample,omitempty"`
}

// Component is a reusable template part, called as {{ component "key" … }}.
type Component struct {
	Key    string
	Source string
	Inputs []Input
}

// Library resolves components by key.
type Library interface {
	Component(key string) (Component, bool)
}

var inputName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateInputs checks input declarations of a component.
func ValidateInputs(inputs []Input) error {
	seen := map[string]bool{}
	for _, in := range inputs {
		if !inputName.MatchString(in.Name) {
			return fmt.Errorf("input name %q must be an identifier (letters, digits, _)", in.Name)
		}
		if seen[in.Name] {
			return fmt.Errorf("input %q is declared twice", in.Name)
		}
		seen[in.Name] = true
		switch in.Kind {
		case "data", "string", "number", "bool":
		default:
			return fmt.Errorf("input %q: kind must be data, string, number or bool", in.Name)
		}
		if reserved[in.Name] {
			return fmt.Errorf("input %q clashes with a built-in field", in.Name)
		}
	}
	return nil
}

var reserved = map[string]bool{"Now": true, "Screen": true, "Region": true, "Device": true, "Data": true, "Stale": true}

const maxComponentDepth = 5

// componentCaller executes components for one region render; it carries the region's context so
// components can use .Now, .Screen, .Region and .Data too.
type componentCaller struct {
	lib    Library
	parent Context
	depth  int
	parsed map[string]*template.Template
}

func (c *componentCaller) funcs() template.FuncMap {
	return template.FuncMap{"component": c.call}
}

// call runs a component. Arguments are either one value (assigned to the first input) or
// name/value pairs: {{ component "forecast" .Data.weather }} or
// {{ component "forecast" "data" .Data.weather "days" 5 }}.
func (c *componentCaller) call(key string, args ...any) (template.HTML, error) {
	if c.lib == nil {
		return "", errors.New("components are not available here")
	}
	comp, ok := c.lib.Component(key)
	if !ok {
		return "", fmt.Errorf("unknown component %q", key)
	}
	if c.depth >= maxComponentDepth {
		return "", fmt.Errorf("component %q: components nested more than %d deep", key, maxComponentDepth)
	}

	values, err := bindInputs(comp, args)
	if err != nil {
		return "", fmt.Errorf("component %q: %w", key, err)
	}
	values["Now"], values["Screen"], values["Region"] = c.parent.Now, c.parent.Screen, c.parent.Region
	values["Device"], values["Data"], values["Stale"] = c.parent.Device, c.parent.Data, c.parent.Stale

	t, ok := c.parsed[key]
	if !ok {
		t, err = template.New(key).Option("missingkey=zero").Funcs(funcs).Funcs(c.funcs()).Parse(comp.Source)
		if err != nil {
			return "", fmt.Errorf("component %q: %w", key, err)
		}
		c.parsed[key] = t
	}
	c.depth++
	defer func() { c.depth-- }()
	var buf bytes.Buffer
	if err := t.Execute(&limitWriter{w: &buf, n: maxOutput}, values); err != nil {
		return "", fmt.Errorf("component %q: %w", key, err)
	}
	return template.HTML(buf.String()), nil
}

func bindInputs(comp Component, args []any) (map[string]any, error) {
	values := map[string]any{}
	for _, in := range comp.Inputs {
		values[in.Name] = in.Default
	}
	switch {
	case len(args) == 0:
	case len(args) == 1:
		if len(comp.Inputs) == 0 {
			return nil, errors.New("takes no inputs")
		}
		values[comp.Inputs[0].Name] = args[0]
	case len(args)%2 == 0:
		known := map[string]bool{}
		for _, in := range comp.Inputs {
			known[in.Name] = true
		}
		for i := 0; i < len(args); i += 2 {
			name, ok := args[i].(string)
			if !ok || !known[name] {
				return nil, fmt.Errorf("unknown input %v (inputs: %s)", args[i], inputNames(comp.Inputs))
			}
			values[name] = args[i+1]
		}
	default:
		return nil, fmt.Errorf("pass one value or name/value pairs (inputs: %s)", inputNames(comp.Inputs))
	}
	return values, nil
}

func inputNames(inputs []Input) string {
	var b bytes.Buffer
	for i, in := range inputs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(in.Name)
	}
	return b.String()
}
