package email

import (
	"embed"
	"fmt"
	"html"
	"net/http"
	"strings"
	texttemplate "text/template"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// All available email template constants.
const (
	// TemplateEmailVerification specifies the new-email-address
	// verification template.
	TemplateEmailVerification Template = "email_verification"

	// TemplateEmailChangeConfirmation specifies the template asking the
	// current address to approve a change of address.
	TemplateEmailChangeConfirmation Template = "email_change_confirmation"

	// TemplateOrganizationInvitation specifies the organization
	// invitation template.
	TemplateOrganizationInvitation Template = "organization_invitation"

	// TemplateUserDeletion specifies the account deletion confirmation
	// template.
	TemplateUserDeletion Template = "user_deletion"

	// TemplatePasswordReset specifies the password reset template.
	TemplatePasswordReset Template = "password_reset"

	// TemplateAccountExists specifies the account-exists notification
	// template.
	TemplateAccountExists Template = "account_exists"

	// TemplateSignupVerification specifies the account-activation
	// template for fresh signups.
	TemplateSignupVerification Template = "signup_verification"
)

// ErrInvalidTemplate is returned when the requested email template is
// not recognized.
var ErrInvalidTemplate = errutil.New(http.StatusBadRequest, "email.invalid_template", "Invalid email template.")

// _specs maps every template to the subject line it is sent under and
// the arguments its HTML file reads. A template absent from the map
// cannot be sent.
var _specs = map[Template]spec{
	TemplateEmailVerification: {
		subject: "Verify your new email address",
		args:    linkArgs,
	},
	TemplateEmailChangeConfirmation: {
		subject: "Approve your email address change",
		args:    linkArgs,
	},
	TemplateOrganizationInvitation: {
		subjectFrom: func(d Data) string { return fmt.Sprintf("You're invited to the %s workspace", d.Organization) },
		args: func(d Data) map[string]string {
			return map[string]string{
				_linkKey:       d.Link,
				"organization": d.Organization,
			}
		},
	},
	TemplateUserDeletion: {
		subject: "Confirm your account deletion",
		args:    linkArgs,
	},
	TemplatePasswordReset: {
		subject: "Reset your password",
		args:    linkArgs,
	},
	TemplateSignupVerification: {
		subject: "Confirm your email address",
		args:    linkArgs,
	},
	TemplateAccountExists: {
		subject: "You already have an Oxynote account",
		args:    linkArgs,
	},
}

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

// Template identifies an email template. Its value matches the base
// name of the embedded HTML file the template is rendered from.
type Template string

// Validate reports whether the template is one that can be sent.
func (t Template) Validate() error {
	if _, ok := _specs[t]; !ok {
		return ErrInvalidTemplate
	}

	return nil
}

// Data carries what a template is rendered with: the recipient and the
// values the templates read. A template ignores the fields it has no
// use for.
type Data struct {
	// Email is the recipient address.
	Email string `json:"email"`

	// Organization is the name of the organization an invitation is to.
	Organization string `json:"organization"`

	// Link is the action URL the email carries.
	Link string `json:"link"`
}

// spec describes how a template is sent: its subject line and the
// arguments its HTML file reads.
type spec struct {
	// subject is the subject line, when it is the same for every send.
	subject string

	// subjectFrom builds the subject line from the data, for a template
	// whose subject names something in it. It takes precedence over
	// subject.
	subjectFrom func(Data) string

	// args builds the template arguments.
	args func(Data) map[string]string
}

// linkArgs builds the arguments of a template that reads only the link.
func linkArgs(d Data) map[string]string {
	return map[string]string{_linkKey: d.Link}
}

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
