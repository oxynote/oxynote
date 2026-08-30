package email

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_render(t *testing.T) {
	cc := map[string]struct {
		Template Template
		Args     map[string]string
		Contains []string
		Err      error
	}{
		"Error returned by unknown template": {
			Template: Template("nonexistent"),
			Err:      assert.AnError,
		},
		"Email verification": {
			Template: TemplateEmailVerification,
			Args: map[string]string{
				"link": "https://example.com/verify?t=abc",
			},
			Contains: []string{
				"https://example.com/verify?t=abc",
			},
		},
		"Organization invitation": {
			Template: TemplateOrganizationInvitation,
			Args: map[string]string{
				"link":         "https://example.com/join?t=abc",
				"organization": "Acme Corp",
			},
			Contains: []string{
				"https://example.com/join?t=abc",
				"Acme Corp",
			},
		},
		"User deletion": {
			Template: TemplateUserDeletion,
			Args: map[string]string{
				"link": "https://example.com/delete?t=abc",
			},
			Contains: []string{
				"https://example.com/delete?t=abc",
			},
		},
		"Password reset": {
			Template: TemplatePasswordReset,
			Args: map[string]string{
				"link": "https://example.com/reset?t=abc",
			},
			Contains: []string{
				"https://example.com/reset?t=abc",
				"Reset your password",
			},
		},
		"Organization invitation escapes args": {
			Template: TemplateOrganizationInvitation,
			Args: map[string]string{
				"link":         "https://example.com/join?a=1&b=2",
				"organization": "Acme & Co <script>",
			},
			Contains: []string{
				"https://example.com/join?a=1&amp;b=2",
				"Acme &amp; Co &lt;script&gt;",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := render(c.Template, c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotEmpty(t, res)

			assert.NotContains(t, res, "{{", "rendered body contains unsubstituted placeholders")

			// the MJML-compiled templates rely on Outlook conditional
			// comments (<!--[if mso]> ghost tables etc.); rendering
			// must not strip them.
			assert.Contains(t, res, "<!--[if", "rendered body lost the Outlook conditional comments")

			assert.Contains(t, res, `src="cid:logo.png"`, "rendered body must reference the embedded logo")
			assert.NotContains(t, res, "cdn.prod.website-files.com", "rendered body references a remote CDN image")

			for _, want := range c.Contains {
				assert.Contains(t, res, want)
			}
		})
	}
}
