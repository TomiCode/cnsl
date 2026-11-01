package connector

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/TomiCode/cnsl/internal/i18n"
)

// Open-Meteo: free weather forecast API, no key needed (https://open-meteo.com).
type openMeteo struct{}

func init() { register(openMeteo{}) }

func (openMeteo) Key() string  { return "openmeteo" }
func (openMeteo) Name() string { return "Open-Meteo weather" }
func (openMeteo) Description() string {
	return "Current weather and a daily forecast for a location. In templates: .Data.<alias>.current.temperature, range .Data.<alias>.daily …"
}

func (openMeteo) ConfigSchema() Schema { return Schema{} }

func (openMeteo) ParamsSchema() Schema {
	return Schema{
		{Key: "latitude", Label: "Latitude", Kind: "number", Required: true, Help: "e.g. 48.2082"},
		{Key: "longitude", Label: "Longitude", Kind: "number", Required: true, Help: "e.g. 16.3738"},
		{Key: "units", Label: "Units", Kind: "select", Options: []string{"metric", "imperial"}, Default: "metric"},
		{Key: "days", Label: "Forecast days", Kind: "int", Default: float64(3), Help: "1–7"},
	}
}

type omResponse struct {
	Current struct {
		Time        string  `json:"time"`
		Temperature float64 `json:"temperature_2m"`
		FeelsLike   float64 `json:"apparent_temperature"`
		Humidity    float64 `json:"relative_humidity_2m"`
		Pressure    float64 `json:"surface_pressure"`
		WindSpeed   float64 `json:"wind_speed_10m"`
		Code        int     `json:"weather_code"`
		IsDay       int     `json:"is_day"`
	} `json:"current"`
	CurrentUnits map[string]string `json:"current_units"`
	Daily        struct {
		Time       []string  `json:"time"`
		Code       []int     `json:"weather_code"`
		Max        []float64 `json:"temperature_2m_max"`
		Min        []float64 `json:"temperature_2m_min"`
		PrecipProb []float64 `json:"precipitation_probability_max"`
		Sunrise    []string  `json:"sunrise"`
		Sunset     []string  `json:"sunset"`
	} `json:"daily"`
	Timezone string `json:"timezone"`
}

func (openMeteo) Fetch(ctx context.Context, env Env, _, params Values) (any, error) {
	days := min(max(params.Int("days", 3), 1), 7)
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(params.Float("latitude", 0), 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(params.Float("longitude", 0), 'f', 4, 64))
	q.Set("current", "temperature_2m,apparent_temperature,relative_humidity_2m,surface_pressure,wind_speed_10m,weather_code,is_day")
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max,sunrise,sunset")
	q.Set("timezone", "auto")
	q.Set("forecast_days", strconv.Itoa(days))
	if params.String("units") == "imperial" {
		q.Set("temperature_unit", "fahrenheit")
		q.Set("wind_speed_unit", "mph")
	}
	var r omResponse
	if err := getJSON(ctx, env.HTTP, "https://api.open-meteo.com/v1/forecast?"+q.Encode(), nil, &r); err != nil {
		return nil, err
	}
	return omData(r), nil
}

func omData(r omResponse) map[string]any {
	c := r.Current
	daily := make([]any, 0, len(r.Daily.Time))
	for i, day := range r.Daily.Time {
		d := map[string]any{"date": day, "code": at(r.Daily.Code, i), "condition": wmoCondition(at(r.Daily.Code, i)),
			"icon": wmoIcon(at(r.Daily.Code, i), true),
			"max":  atF(r.Daily.Max, i), "min": atF(r.Daily.Min, i), "precipitation_probability": atF(r.Daily.PrecipProb, i),
			"sunrise": atS(r.Daily.Sunrise, i), "sunset": atS(r.Daily.Sunset, i)}
		if t, err := time.Parse("2006-01-02", day); err == nil {
			d["weekday"] = t.Weekday().String()
		}
		daily = append(daily, d)
	}
	return map[string]any{
		"current": map[string]any{
			"time": c.Time, "temperature": c.Temperature, "feels_like": c.FeelsLike, "humidity": c.Humidity,
			"pressure": c.Pressure, "wind_speed": c.WindSpeed, "code": c.Code, "condition": wmoCondition(c.Code),
			"icon":   wmoIcon(c.Code, c.IsDay == 1),
			"is_day": c.IsDay == 1,
		},
		"units": map[string]any{
			"temperature": r.CurrentUnits["temperature_2m"], "wind_speed": r.CurrentUnits["wind_speed_10m"],
			"pressure": r.CurrentUnits["surface_pressure"],
		},
		"daily":    daily,
		"timezone": r.Timezone,
	}
}

func at(s []int, i int) int {
	if i < len(s) {
		return s[i]
	}
	return 0
}
func atF(s []float64, i int) float64 {
	if i < len(s) {
		return s[i]
	}
	return 0
}
func atS(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

// wmoCondition names WMO weather interpretation codes (as used by Open-Meteo).
// wmoCondition is the English name of a WMO weather code; templates translate with
// {{ condition .code }}.
func wmoCondition(code int) string { return i18n.Get("en").Condition(code) }

// wmoIcon maps a WMO weather code to a Weather Icons name (see internal/icons).
func wmoIcon(code int, day bool) string {
	pick := func(d, n string) string {
		if day {
			return "wi-" + d
		}
		return "wi-" + n
	}
	switch {
	case code == 0:
		return pick("day-sunny", "night-clear")
	case code == 1:
		return pick("day-sunny-overcast", "night-alt-partly-cloudy")
	case code == 2:
		return pick("day-cloudy", "night-alt-cloudy")
	case code == 3:
		return "wi-cloudy"
	case code == 45 || code == 48:
		return pick("day-fog", "night-fog")
	case code == 51 || code == 53 || code == 55:
		return pick("day-sprinkle", "night-alt-sprinkle")
	case code == 56 || code == 57:
		return pick("day-sleet", "night-alt-sleet")
	case code == 61 || code == 63 || code == 65:
		return pick("day-rain", "night-alt-rain")
	case code == 66 || code == 67:
		return pick("day-rain-mix", "night-alt-rain-mix")
	case code >= 71 && code <= 75, code == 85, code == 86:
		return pick("day-snow", "night-alt-snow")
	case code == 77:
		return "wi-snowflake-cold"
	case code >= 80 && code <= 82:
		return pick("day-showers", "night-alt-showers")
	case code == 95:
		return pick("day-thunderstorm", "night-alt-thunderstorm")
	case code == 96 || code == 99:
		return pick("day-hail", "night-alt-hail")
	}
	return "wi-na"
}

func (openMeteo) Sample(params Values) any {
	var r omResponse
	r.Current.Time = time.Now().Format("2006-01-02T15:04")
	r.Current.Temperature, r.Current.FeelsLike, r.Current.Humidity = 14.2, 12.8, 61
	r.Current.Pressure, r.Current.WindSpeed, r.Current.Code, r.Current.IsDay = 1013.4, 11.5, 2, 1
	r.CurrentUnits = map[string]string{"temperature_2m": "°C", "wind_speed_10m": "km/h", "surface_pressure": "hPa"}
	codes := []int{2, 61, 0, 3, 80, 1, 71}
	for i := range min(max(params.Int("days", 3), 1), 7) {
		day := time.Now().AddDate(0, 0, i).Format("2006-01-02")
		r.Daily.Time = append(r.Daily.Time, day)
		r.Daily.Code = append(r.Daily.Code, codes[i])
		r.Daily.Max = append(r.Daily.Max, 16+float64(i))
		r.Daily.Min = append(r.Daily.Min, 7+float64(i)/2)
		r.Daily.PrecipProb = append(r.Daily.PrecipProb, float64(10*i))
		r.Daily.Sunrise = append(r.Daily.Sunrise, day+"T06:45")
		r.Daily.Sunset = append(r.Daily.Sunset, day+"T16:50")
	}
	r.Timezone = "Europe/Vienna"
	return omData(r)
}
