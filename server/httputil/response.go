package httputil

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
)

// WriteErrorJSON writes a JSON error response using the Mattermost AppError
// format.
func WriteErrorJSON(w http.ResponseWriter, statusCode int, message string, detailedError string) {
	resp := mmmodel.AppError{
		Id:            errorIDFromStatus(statusCode),
		Message:       message,
		DetailedError: detailedError,
		StatusCode:    statusCode,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(&resp)
}

// errorIDFromStatus derives a plugin-scoped error ID from an HTTP status code.
// For example, 400 → "plugin.channel_automation.bad_request".
var nonAlpha = regexp.MustCompile(`[^a-z0-9]+`)

func errorIDFromStatus(code int) string {
	text := http.StatusText(code)
	if text == "" {
		text = "Error"
	}
	id := strings.TrimRight(nonAlpha.ReplaceAllString(strings.ToLower(text), "_"), "_")
	return "plugin.channel_automation." + id
}
