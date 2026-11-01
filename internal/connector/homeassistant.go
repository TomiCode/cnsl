package connector

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Home Assistant: current states of selected entities via the REST API (GET /api/states).
type homeAssistant struct{}

func init() { register(homeAssistant{}) }

func (homeAssistant) Key() string  { return "homeassistant" }
func (homeAssistant) Name() string { return "Home Assistant" }
func (homeAssistant) Description() string {
	return "Current state of sensors and other entities. In templates: .Data.<alias>.<entity_id with _ instead of .>, e.g. .Data.ha.sensor_outdoor_temperature.state"
}

func (homeAssistant) ConfigSchema() Schema {
	return Schema{
		{Key: "url", Label: "Home Assistant URL", Kind: "url", Required: true, Help: "e.g. http://homeassistant.local:8123"},
		{Key: "token", Label: "Long-lived access token", Kind: "secret", Required: true, Help: "Profile → Security → Long-lived access tokens"},
	}
}

func (homeAssistant) ParamsSchema() Schema {
	return Schema{
		{Key: "entities", Label: "Entities", Kind: "list", Required: true, Help: "One entity id per line, e.g. sensor.outdoor_temperature"},
	}
}

type haState struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged string         `json:"last_changed"`
	LastUpdated string         `json:"last_updated"`
}

func (homeAssistant) Fetch(ctx context.Context, env Env, config, params Values) (any, error) {
	var states []haState
	err := getJSON(ctx, env.HTTP, config.String("url")+"/api/states",
		map[string]string{"Authorization": "Bearer " + config.String("token")}, &states)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]haState, len(states))
	for _, s := range states {
		byID[s.EntityID] = s
	}

	out := map[string]any{}
	var missing []string
	for _, id := range params.List("entities") {
		s, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		out[templateKey(id)] = haEntity(s)
	}
	if len(missing) > 0 && len(out) == 0 {
		return nil, fmt.Errorf("unknown entities: %s", strings.Join(missing, ", "))
	}
	if len(missing) > 0 {
		out["_missing"] = missing
	}
	return out, nil
}

func haEntity(s haState) map[string]any {
	e := map[string]any{
		"entity_id":    s.EntityID,
		"state":        s.State,
		"name":         s.Attributes["friendly_name"],
		"unit":         s.Attributes["unit_of_measurement"],
		"attributes":   s.Attributes,
		"last_changed": s.LastChanged,
	}
	e["numeric"] = false
	if v, err := strconv.ParseFloat(s.State, 64); err == nil {
		e["value"], e["numeric"] = v, true // numeric states, ready for num/round
	}
	return e
}

// templateKey makes an entity id usable as a template field: sensor.outdoor_temp → sensor_outdoor_temp.
func templateKey(entityID string) string {
	return strings.NewReplacer(".", "_", "-", "_").Replace(entityID)
}

func (homeAssistant) Sample(params Values) any {
	out := map[string]any{}
	entities := params.List("entities")
	if len(entities) == 0 {
		entities = []string{"sensor.outdoor_temperature", "sensor.pressure"}
	}
	for i, id := range entities {
		v := 21.5 + float64(i)*3.25
		unit := "°C"
		if strings.Contains(id, "pressure") {
			v, unit = 1013.2, "hPa"
		} else if strings.Contains(id, "humidity") {
			v, unit = 48, "%"
		}
		out[templateKey(id)] = haEntity(haState{
			EntityID:   id,
			State:      strconv.FormatFloat(v, 'f', -1, 64),
			Attributes: map[string]any{"friendly_name": strings.ReplaceAll(id[strings.IndexByte(id, '.')+1:], "_", " "), "unit_of_measurement": unit},
		})
	}
	return out
}
