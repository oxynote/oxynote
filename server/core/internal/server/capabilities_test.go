package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Server_fetchCapabilities(t *testing.T) {
	t.Parallel()

	s := Server{
		log: discardLog(),
		capabilities: Capabilities{
			Slack:  true,
			Search: true,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
	rec := httptest.NewRecorder()

	s.fetchCapabilities(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(
		t,
		`{"github":false,"slack":true,"assistant":false,"changedetection":false,"search":true}`,
		rec.Body.String(),
	)
}
