package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestServeHTTP(t *testing.T) {
	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		plugin := Plugin{}
		router := mux.NewRouter()
		router.Use(plugin.MattermostAuthorizationRequired)
		router.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		plugin.router = router

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/test", nil)

		plugin.ServeHTTP(nil, w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("unknown route returns 404", func(t *testing.T) {
		plugin := Plugin{}
		plugin.router = mux.NewRouter()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)

		plugin.ServeHTTP(nil, w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
