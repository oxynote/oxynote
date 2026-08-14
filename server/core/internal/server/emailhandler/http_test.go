package emailhandler

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
			assert.Equal(t, code, rec.Code)

			if body == "" {
				assert.Zero(t, rec.Body.Len(), rec.Body.String())
				return
			}

			assert.JSONEq(t, body, rec.Body.String())
		}
	}

	wasNoSendCalled := func() check {
		return func(t *testing.T, sender *SenderMock, _ *httptest.ResponseRecorder) {
			assert.Empty(t, sender.SendEmailVerificationCalls())
			assert.Empty(t, sender.SendPasswordResetCalls())
			assert.Empty(t, sender.SendAccountExistsCalls())
			assert.Empty(t, sender.SendSignupVerificationCalls())
			assert.Empty(t, sender.SendOrganizationInvitationCalls())
			assert.Empty(t, sender.SendUserDeletionConfirmationCalls())
			assert.Empty(t, sender.SendUserCreationCalls())
		}
	}

	wasSendEmailVerificationCalled := func(count int, eml, link string) check {
		return func(t *testing.T, sender *SenderMock, _ *httptest.ResponseRecorder) {
			ff := sender.SendEmailVerificationCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, eml, ff[0].Eml)
			assert.Equal(t, link, ff[0].Link)
		}
	}

	wasSendPasswordResetCalled := func(count int, eml, link string) check {
		return func(t *testing.T, sender *SenderMock, _ *httptest.ResponseRecorder) {
			ff := sender.SendPasswordResetCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, eml, ff[0].Eml)
			assert.Equal(t, link, ff[0].Link)
		}
	}

	wasSendAccountExistsCalled := func(count int, eml, link string) check {
		return func(t *testing.T, sender *SenderMock, _ *httptest.ResponseRecorder) {
			ff := sender.SendAccountExistsCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, eml, ff[0].Eml)
			assert.Equal(t, link, ff[0].Link)
		}
	}

	wasSendSignupVerificationCalled := func(count int, eml, link string) check {
		return func(t *testing.T, sender *SenderMock, _ *httptest.ResponseRecorder) {
			ff := sender.SendSignupVerificationCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, eml, ff[0].Eml)
			assert.Equal(t, link, ff[0].Link)
		}
	}

	wasSendOrganizationInvitationCalled := func(count int, eml, org, link string) check {
		return func(t *testing.T, sender *SenderMock, _ *httptest.ResponseRecorder) {
			ff := sender.SendOrganizationInvitationCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, eml, ff[0].Eml)
			assert.Equal(t, org, ff[0].Org)
			assert.Equal(t, link, ff[0].Link)
		}
	}

	wasSendUserDeletionConfirmationCalled := func(count int, eml, link string) check {
		return func(t *testing.T, sender *SenderMock, _ *httptest.ResponseRecorder) {
			ff := sender.SendUserDeletionConfirmationCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, eml, ff[0].Eml)
			assert.Equal(t, link, ff[0].Link)
		}
	}

	wasSendUserCreationCalled := func(count int, eml string) check {
		return func(t *testing.T, sender *SenderMock, _ *httptest.ResponseRecorder) {
			ff := sender.SendUserCreationCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, eml, ff[0].Eml)
		}
	}

	cc := map[string]struct {
		Body   string
		Checks []check
	}{
		"Invalid JSON body": {
			Body: "{",
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"request.invalid_json","message":"invalid JSON body"}`),
				wasNoSendCalled(),
			),
		},
		"Unknown template": {
			Body: `{"template":"nonexistent","data":{"email":"user@example.com"}}`,
			Checks: checks(
				hasResp(http.StatusBadRequest, `{"code":"email.invalid_template","message":"Invalid email template."}`),
				wasNoSendCalled(),
			),
		},
		"Email verification sent": {
			Body: `{"template":"email_verification","data":{"email":"user@example.com","link":"https://example.com/verify"}}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasSendEmailVerificationCalled(1, "user@example.com", "https://example.com/verify"),
			),
		},
		"Password reset sent": {
			Body: `{"template":"password_reset","data":{"email":"user@example.com","link":"https://example.com/reset"}}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasSendPasswordResetCalled(1, "user@example.com", "https://example.com/reset"),
			),
		},
		"Account exists sent": {
			Body: `{"template":"account_exists","data":{"email":"user@example.com","link":"https://example.com/login"}}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasSendAccountExistsCalled(1, "user@example.com", "https://example.com/login"),
			),
		},
		"Signup verification sent": {
			Body: `{"template":"signup_verification","data":{"email":"user@example.com","link":"https://example.com/activate"}}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasSendSignupVerificationCalled(1, "user@example.com", "https://example.com/activate"),
			),
		},
		"Organization invitation sent": {
			Body: `{"template":"organization_invitation","data":{"email":"user@example.com","organization":"Acme","link":"https://example.com/join"}}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasSendOrganizationInvitationCalled(1, "user@example.com", "Acme", "https://example.com/join"),
			),
		},
		"User deletion confirmation sent": {
			Body: `{"template":"user_deletion","data":{"email":"user@example.com","link":"https://example.com/delete"}}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasSendUserDeletionConfirmationCalled(1, "user@example.com", "https://example.com/delete"),
			),
		},
		"User creation sent": {
			Body: `{"template":"user_creation","data":{"email":"user@example.com"}}`,
			Checks: checks(
				hasResp(http.StatusNoContent, ""),
				wasSendUserCreationCalled(1, "user@example.com"),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			sender := &SenderMock{}

			hdl := Handler{
				log:    slog.New(slog.DiscardHandler),
				sender: sender,
			}

			req := httptest.NewRequest("POST", "http://test.com/", strings.NewReader(c.Body))
			rec := httptest.NewRecorder()

			hdl.SendEmail(rec, req)

			for _, ch := range c.Checks {
				ch(t, sender, rec)
			}
		})
	}
}
