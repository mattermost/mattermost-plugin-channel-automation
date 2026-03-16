package httputil

import (
	"encoding/json"
	"net/http"
)

// WriteErrorJSON writes a JSON error response with the given status code.
// The response body is {"error": message, ...extras}. Optional extra fields
// are provided as key-value string/any pairs that are merged into the object.
func WriteErrorJSON(w http.ResponseWriter, statusCode int, message string, extras ...any) {
	resp := map[string]any{"error": message}
	for i := 0; i+1 < len(extras); i += 2 {
		if key, ok := extras[i].(string); ok {
			resp[key] = extras[i+1]
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}
