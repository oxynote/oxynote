package email

import (
	"embed"
	"fmt"
	"html"
	"strings"
	texttemplate "text/template"
)

// A list of email templates.
const (
	_templateEmailVerification      template = "email_verification"
	_templateOrganizationInvitation template = "organization_invitation"
	_templateUserDeletion           template = "user_deletion"
	_templateUserCreation           template = "user_creation"
	_templatePasswordReset          template = "password_reset"
	_templateAccountExists          template = "account_exists"
	_templateSignupVerification     template = "signup_verification"
)

// template is used to identify template.
type template string

//go:embed templates/*.html
var _templateFS embed.FS

// _logoPNG is the logo the templates reference as "cid:logo.png". It is
// attached inline to every email: remote images get blocked by many
// clients and data: URIs don't render in Gmail, so a CID attachment is
// the only reliable way to show it.
//
//go:embed assets/logo.png
var _logoPNG []byte

// _templates holds all email templates parsed from the embedded HTML
// files, addressable by "<template>.html".
// text/template is used instead of html/template on purpose: the
// html/template sanitizer elides all HTML comments, which would strip
// the Outlook conditional comments (<!--[if mso]> DPI fixes and ghost
// tables) the MJML-compiled templates rely on. The templates are static
// trusted files; render HTML-escapes every dynamic argument instead.
var _templates = texttemplate.Must(
	texttemplate.ParseFS(_templateFS, "templates/*.html"),
)

// render executes the specified template with the provided arguments,
// HTML-escaping each argument value, and returns the resulting HTML
// body.
func render(tmpl template, args map[string]string) (string, error) {
	escaped := make(map[string]string, len(args))

	for k, v := range args {
		escaped[k] = html.EscapeString(v)
	}

	var sb strings.Builder

	err := _templates.ExecuteTemplate(&sb, string(tmpl)+".html", escaped)
	if err != nil {
		return "", fmt.Errorf("execute template %q: %w", tmpl, err)
	}

	return sb.String(), nil
}
