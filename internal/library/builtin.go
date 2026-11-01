package library

import (
	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/internal/tmpl"
)

type builtin struct {
	Key, Name, Description string
	Inputs                 []tmpl.Input
}

// Sources live in components/<key>.html.
var builtinComponents = []builtin{
	{"weather-current", "Current weather", "Temperature and condition from an Open-Meteo binding, with optional details.",
		[]tmpl.Input{
			{Name: "weather", Label: "Weather data", Kind: "data", Connector: "openmeteo", Help: "An Open-Meteo binding, e.g. .Data.weather"},
			{Name: "details", Label: "Show details", Kind: "bool", Default: true, Help: "Feels like, humidity and wind"},
		}},
	{"weather-forecast", "Weather forecast", "Daily highs, lows and conditions from an Open-Meteo binding.",
		[]tmpl.Input{
			{Name: "weather", Label: "Weather data", Kind: "data", Connector: "openmeteo"},
			{Name: "days", Label: "Days", Kind: "number", Default: 3, Help: "The binding fetches up to its own day count"},
			{Name: "rain", Label: "Show rain chance", Kind: "bool", Default: false},
		}},
	{"sensor", "Sensor", "One Home Assistant entity as a value with a label.",
		[]tmpl.Input{
			{Name: "entity", Label: "Entity", Kind: "data", Connector: "homeassistant",
				Help: "One entity of a Home Assistant binding, e.g. .Data.ha.sensor_outdoor_temperature",
				Sample: map[string]any{"entity_id": "sensor.outdoor_temperature", "state": "21.5", "value": 21.5, "numeric": true,
					"name": "Outdoor temperature", "unit": "°C"}},
			{Name: "label", Label: "Label", Kind: "string", Help: "Defaults to the entity's name"},
			{Name: "places", Label: "Decimals", Kind: "number", Default: 1},
		}},
	{"ha-sensors", "Sensor table", "All entities of a Home Assistant binding as a table.",
		[]tmpl.Input{
			{Name: "entities", Label: "Entities", Kind: "data", Connector: "homeassistant", Help: "A Home Assistant binding, e.g. .Data.ha"},
			{Name: "places", Label: "Decimals", Kind: "number", Default: 1},
		}},
	{"status-list", "Status list", "Up/down checks from a status binding.",
		[]tmpl.Input{
			{Name: "status", Label: "Status data", Kind: "data", Connector: "status"},
			{Name: "latency", Label: "Show latency", Kind: "bool", Default: true},
		}},
	{"clock", "Clock", "Time and date of the render (devices refresh on their own schedule, so pick a coarse format).",
		[]tmpl.Input{
			{Name: "time_format", Label: "Time format", Kind: "string", Default: "15:04", Help: "Go layout: 15:04, 3:04 PM"},
			{Name: "date_format", Label: "Date format", Kind: "string", Default: "Monday, 2 January", Help: "Go layout; empty hides the date"},
		}},
	{"month-grid", "Month calendar", "The current month, Monday first, with today highlighted.",
		[]tmpl.Input{
			{Name: "title", Label: "Show month name", Kind: "bool", Default: true},
		}},
	{"title-bar", "Title bar", "Framework title bar, placed at the bottom of a view.",
		[]tmpl.Input{
			{Name: "title", Label: "Title", Kind: "string", Help: "Defaults to the screen's name"},
			{Name: "instance", Label: "Instance", Kind: "string", Help: "Small text after the title"},
		}},
}

// Snippet bodies use Monaco snippet syntax ($1, ${1:default}, $0).
var builtinSnippets = []store.Snippet{
	{Prefix: "view", Name: "View with title bar", Description: "Full view with a centered layout and title bar (legacy regions that bring their own view)",
		Body: "<div class=\"view view--full\">\n  <div class=\"layout layout--col layout--center gap--large\">\n    $0\n  </div>\n  <div class=\"title_bar\">\n    <span class=\"title\">${1:{{ .Screen.Name }}}</span>\n  </div>\n</div>"},
	{Prefix: "layout", Name: "Layout", Description: "Centered column layout filling the region",
		Body: "<div class=\"layout layout--col layout--center gap--medium\">\n  $0\n</div>"},
	{Prefix: "title_bar", Name: "Title bar", Description: "Framework title bar",
		Body: "<div class=\"title_bar\">\n  <span class=\"title\">$1</span>\n  <span class=\"instance\">$2</span>\n</div>"},
	{Prefix: "item", Name: "Item", Description: "Framework item (meta + content)",
		Body: "<div class=\"item\">\n  <div class=\"meta\"></div>\n  <div class=\"content\">\n    <span class=\"title title--small\">$1</span>\n    <span class=\"description\">$2</span>\n  </div>\n</div>"},
	{Prefix: "value", Name: "Value", Description: "Big value with a label",
		Body: "<span class=\"value value--xlarge value--tnums\">$1</span>\n<span class=\"label\">$2</span>"},
	{Prefix: "columns", Name: "Columns", Description: "Two columns",
		Body: "<div class=\"columns\">\n  <div class=\"column\">$1</div>\n  <div class=\"column\">$2</div>\n</div>"},
	{Prefix: "grid", Name: "Grid", Description: "Grid with N columns",
		Body: "<div class=\"grid grid--cols-${1:3}\">\n  $0\n</div>"},
	{Prefix: "table", Name: "Table", Description: "Table over a list",
		Body: "<table class=\"table\">\n  <thead>\n    <tr><th><span class=\"title title--small\">$1</span></th></tr>\n  </thead>\n  <tbody>\n  {{ range ${2:.Data.items} }}\n    <tr><td><span class=\"label\">$0</span></td></tr>\n  {{ end }}\n  </tbody>\n</table>"},
	{Prefix: "range", Name: "range", Description: "Go template range",
		Body: "{{ range ${1:.Data.items} }}\n  $0\n{{ end }}"},
	{Prefix: "if", Name: "if", Description: "Go template if",
		Body: "{{ if ${1:condition} }}\n  $0\n{{ end }}"},
	{Prefix: "with", Name: "with", Description: "Go template with/else (handles missing data)",
		Body: "{{ with ${1:.Data.alias} }}\n  $0\n{{ else }}\n  <span class=\"label\">No data</span>\n{{ end }}"},
	{Prefix: "date", Name: "date", Description: "Format the current time",
		Body: "{{ date \"${1:Monday, 2 January}\" .Now }}"},
	{Prefix: "component", Name: "component", Description: "Call a component",
		Body: "{{ component \"${1:key}\" $0}}"},
}
