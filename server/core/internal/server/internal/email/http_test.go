package email

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	emailCore "github.com/oxynote/oxynote/server/core/internal/email"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewHandler(t *testing.T) {
	t.Parallel()

	sender := &SenderMock{}

	hdl := NewHandler(slog.New(slog.DiscardHandler), sender)
	require.NotNil(t, hdl)
	assert.NotNil(t, hdl.log)
	assert.Same(t, sender, hdl.sender)
}

func Test_Handler_SendEmail(t *testing.T) {
	type check func(*testing.T, *SenderMock, *httptest.ResponseRecorder)

	checks := func(cc ...check) []check { return cc }

	hasResp := func(code int, body string) check {
		return func(t *testing.T, _ *SenderMock, rec *httptest.ResponseRecorder) {
			testutil.AssertResponse(t, rec, code, body)
		}
	}

	wasSendCalled := func(count int, tmpl emailCore.Template, d emailCore.Data) check {
		return func(t *testing.T, sender *SenderMock, _ *httptest.ResponseRecorder) {
			ff := sender.SendCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, tmpl, ff[0].Tmpl)
			assert.Equal(t, d, ff[0].D)
		}
	}

	cc := map[string]struct {
		Sender *SenderMock
		Body   string
		Checks []check
	}{
		"Invalid JSON body": {
			Sender: &SenderMock{},
			Body:   "{",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasSendCalled(0, "", emailCore.Data{}),
			),
		},
		"Error returned by Sender.Send": {
			Sender: &SenderMock{
				SendFunc: func(emailCore.Template, emailCore.Data) error {
					return emailCore.ErrInvalidTemplate
				},
			},
			Body: `{"template":"nonexistent","data":{"email":"user@example.com"}}`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"email.invalid_template","message":"Invalid email template."}`),
				wasSendCalled(1, "nonexistent", emailCore.Data{Email: "user@example.com"}),
			),
		},
		"Email sent": {
			Sender: &SenderMock{},
			Body:   `{"template":"organization_invitation","data":{"email":"user@example.com","organization":"Acme","link":"https://example.com/join"}}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasSendCalled(1, emailCore.TemplateOrganizationInvitation, emailCore.Data{
					Email:        "user@example.com",
					Organization: "Acme",
					Link:         "https://example.com/join",
				}),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hdl := Handler{
				log:    slog.New(slog.DiscardHandler),
				sender: c.Sender,
			}

			req := httptest.NewRequest("POST", "http://test.com/", strings.NewReader(c.Body))
			rec := httptest.NewRecorder()

			hdl.SendEmail(rec, req)

			for _, ch := range c.Checks {
				ch(t, c.Sender, rec)
			}
		})
	}
}
