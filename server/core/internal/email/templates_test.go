package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_render(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Template       template
		Args           map[string]string
		WantContains   []string
		WantSourceFile string
		WantErr        bool
	}{
		"Email verification": {
			Template: _templateEmailVerification,
			Args: map[string]string{
				"link": "https://example.com/verify?t=abc",
			},
			WantContains: []string{
				"https://example.com/verify?t=abc",
			},
		},
		"Organization invitation": {
			Template: _templateOrganizationInvitation,
			Args: map[string]string{
				"link":         "https://example.com/join?t=abc",
				"organization": "Acme Corp",
			},
			WantContains: []string{
				"https://example.com/join?t=abc",
				"Acme Corp",
			},
		},
		"User deletion": {
			Template: _templateUserDeletion,
			Args: map[string]string{
				"link": "https://example.com/delete?t=abc",
			},
			WantContains: []string{
				"https://example.com/delete?t=abc",
			},
		},
		"User creation matches source byte-for-byte": {
			Template: _templateUserCreation,
			Args:     nil,
			// no placeholders, so the output must equal the embedded
			// source file exactly, proving rendering does not alter
			// the template (e.g. by stripping comments).
			WantSourceFile: "templates/user_creation.html",
		},
		"Organization invitation escapes args": {
			Template: _templateOrganizationInvitation,
			Args: map[string]string{
				"link":         "https://example.com/join?a=1&b=2",
				"organization": "Acme & Co <script>",
			},
			WantContains: []string{
				"https://example.com/join?a=1&amp;b=2",
				"Acme &amp; Co &lt;script&gt;",
			},
		},
		"Unknown template": {
			Template: template("nonexistent"),
			WantErr:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := render(tc.Template, tc.Args)

			if tc.WantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, got)

			assert.NotContains(t, got, "{{", "rendered body contains unsubstituted placeholders")

			// the MJML-compiled templates rely on Outlook conditional
			// comments (<!--[if mso]> ghost tables etc.); rendering
			// must not strip them.
			assert.Contains(t, got, "<!--[if", "rendered body lost the Outlook conditional comments")

			for _, want := range tc.WantContains {
				assert.Contains(t, got, want)
			}

			if tc.WantSourceFile != "" {
				src, err := _templateFS.ReadFile(tc.WantSourceFile)
				require.NoError(t, err)

				assert.Equal(t, string(src), got)
			}
		})
	}
}
