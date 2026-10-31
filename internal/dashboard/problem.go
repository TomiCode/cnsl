package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
)

// writeValidationProblem turns a spec validation error into a short problem with per-field
// messages, instead of kin-openapi's full error text (which embeds the whole schema).
func writeValidationProblem(w http.ResponseWriter, status int, err error) {
	var reqErr *openapi3filter.RequestError
	if !errors.As(err, &reqErr) {
		writeProblem(w, status, err.Error())
		return
	}

	field, message := "", reqErr.Reason
	if reqErr.Parameter != nil {
		field = reqErr.Parameter.Name
	}
	var schemaErr *openapi3.SchemaError
	if errors.As(reqErr.Err, &schemaErr) {
		if path := schemaErr.JSONPointer(); len(path) > 0 {
			field = strings.Join(path, ".")
		}
		message = schemaErr.Reason
	} else if message == "" && reqErr.Err != nil {
		message = reqErr.Err.Error()
	}

	fe := gen.FieldError{Message: message}
	if field != "" {
		fe.Field = &field
	}
	writeProblem(w, status, "the request does not match the API schema", fe)
}

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
