package main

import (
	"net/http"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
)

func (p *Plugin) MattermostAuthorizationRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("Mattermost-User-ID")
		if userID == "" {
			p.API.LogWarn("Unauthorized request: missing Mattermost-User-ID header", "method", r.Method, "path", r.URL.Path)
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (p *Plugin) SystemAdminRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("Mattermost-User-ID")

		if !p.client.User.HasPermissionTo(userID, mmmodel.PermissionManageSystem) {
			p.API.LogWarn("System admin permission denied", "user_id", userID, "method", r.Method, "path", r.URL.Path)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
