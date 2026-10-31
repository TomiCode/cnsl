package dashboard

import (
	"encoding/json"
	"net/http"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
)

// writeProblem writes an RFC 9457 problem details response.
func writeProblem(w http.ResponseWriter, status int, detail string, fieldErrors ...gen.FieldError) {
	p := gen.Problem{
		Title:  http.StatusText(status),
		Status: status,
	}
	if detail != "" {
		p.Detail = &detail
	}
	if len(fieldErrors) > 0 {
		p.Errors = &fieldErrors
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}
