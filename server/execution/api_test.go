package execution

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLimit(t *testing.T) {
	t.Run("default when missing", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions", nil)
		assert.Equal(t, defaultLimit, parseLimit(r))
	})

	t.Run("valid value", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=50", nil)
		assert.Equal(t, 50, parseLimit(r))
	})

	t.Run("capped at max", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=999999999", nil)
		assert.Equal(t, maxLimit, parseLimit(r))
	})

	t.Run("exactly max", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=100", nil)
		assert.Equal(t, maxLimit, parseLimit(r))
	})

	t.Run("zero falls back to default", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=0", nil)
		assert.Equal(t, defaultLimit, parseLimit(r))
	})

	t.Run("negative falls back to default", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=-5", nil)
		assert.Equal(t, defaultLimit, parseLimit(r))
	})

	t.Run("non-numeric falls back to default", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/executions?limit=abc", nil)
		assert.Equal(t, defaultLimit, parseLimit(r))
	})
}
