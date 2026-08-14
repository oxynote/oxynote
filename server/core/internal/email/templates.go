package email

import (
	"embed"
	"fmt"
	"html"
	"strings"
	texttemplate "text/template"
)

// All available email template constants.
const (
	// TemplateEmailVerification specifies the new-email-address
	// verification template.
	TemplateEmailVerification Template = "email_verification"

	// TemplateOrganizationInvitation specifies the organization
	// invitation template.
	TemplateOrganizationInvitation Template = "organization_invitation"

	// TemplateUserDeletion specifies the account deletion confirmation
	// template.
	TemplateUserDeletion Template = "user_deletion"

	// TemplateUserCreation specifies the welcome template for newly
	// registered users.
	TemplateUserCreation Template = "user_creation"

	// TemplatePasswordReset specifies the password reset template.
	TemplatePasswordReset Template = "password_reset"

	// TemplateAccountExists specifies the account-exists notification
	// template.
	TemplateAccountExists Template = "account_exists"

	// TemplateSignupVerification specifies the account-activation
	// template for fresh signups.
	TemplateSignupVerification Template = "signup_verification"
)

// Template identifies an email template. Its value matches the base
// name of the embedded HTML file the template is rendered from.
type Template string

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
func render(tmpl Template, args map[string]string) (string, error) {
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
