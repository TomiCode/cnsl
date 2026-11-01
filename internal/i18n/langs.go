package i18n

import "fmt"

var english = Lang{
	Code: "en", Name: "English",
	Weekdays:       [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	WeekdaysShort:  [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
	WeekdaysMin:    [7]string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"},
	Months:         [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	MonthsGenitive: [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	MonthsShort:    [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	Ago:            "%s ago", Until: "in %s",
	Units: func(n int, unit string) string {
		if unit == "days" && n == 1 {
			unit = "day"
		}
		return fmt.Sprintf("%d %s", n, unit)
	},
	conditions: [12]string{"Clear", "Mainly clear", "Partly cloudy", "Overcast", "Fog", "Drizzle", "Rain", "Snow", "Showers", "Snow showers", "Thunderstorm"},
	words: map[string]string{
		"feels_like":  "Feels",
		"humidity":    "Humidity",
		"wind":        "Wind",
		"pressure":    "Pressure",
		"rain_chance": "Rain",
		"no_data":     "No data",
		"up":          "up",
		"down":        "DOWN",
		"n_of_m_up":   "%v of %v up",
		"today":       "Today",
		"tomorrow":    "Tomorrow",
		"code":        "Code",
	},
}

var polish = Lang{
	Code: "pl", Name: "Polski",
	Weekdays:       [7]string{"niedziela", "poniedziałek", "wtorek", "środa", "czwartek", "piątek", "sobota"},
	WeekdaysShort:  [7]string{"niedz.", "pon.", "wt.", "śr.", "czw.", "pt.", "sob."},
	WeekdaysMin:    [7]string{"Nd", "Pn", "Wt", "Śr", "Cz", "Pt", "So"},
	Months:         [12]string{"styczeń", "luty", "marzec", "kwiecień", "maj", "czerwiec", "lipiec", "sierpień", "wrzesień", "październik", "listopad", "grudzień"},
	MonthsGenitive: [12]string{"stycznia", "lutego", "marca", "kwietnia", "maja", "czerwca", "lipca", "sierpnia", "września", "października", "listopada", "grudnia"},
	MonthsShort:    [12]string{"sty", "lut", "mar", "kwi", "maj", "cze", "lip", "sie", "wrz", "paź", "lis", "gru"},
	Capitalize:     true,
	Ago:            "%s temu", Until: "za %s",
	Units: func(n int, unit string) string {
		switch unit {
		case "h":
			unit = "godz."
		case "days":
			unit = "dni"
			if n == 1 {
				unit = "dzień"
			}
		}
		return fmt.Sprintf("%d %s", n, unit)
	},
	conditions: [12]string{"Bezchmurnie", "Przeważnie bezchmurnie", "Częściowe zachmurzenie", "Pochmurno", "Mgła", "Mżawka", "Deszcz", "Śnieg", "Przelotne opady", "Przelotny śnieg", "Burza"},
	words: map[string]string{
		"feels_like":  "Odczuwalna",
		"humidity":    "Wilgotność",
		"wind":        "Wiatr",
		"pressure":    "Ciśnienie",
		"rain_chance": "Opady",
		"no_data":     "Brak danych",
		"up":          "działa",
		"down":        "NIE DZIAŁA",
		"n_of_m_up":   "%v z %v działa",
		"today":       "Dziś",
		"tomorrow":    "Jutro",
		"code":        "Kod",
	},
}
