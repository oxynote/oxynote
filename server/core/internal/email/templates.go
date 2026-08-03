package email

// A list of email templates.
const (
	_templateEmailVerification      template = "email_verification"
	_templateOrganizationInvitation template = "organization_invitation"
	_templateUserDeletion           template = "user_deletion"
	_templateUserCreation           template = "user_creation"
)

// template is used to identify template.
type template string
