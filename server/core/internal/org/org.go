// Package org defines organization-wide rules: the roles a member may hold,
// the storage layout of the logo and the URL the logo is served under.
package org

import "fmt"

// _logoFolderFormat is the storage folder format for an organization's
// logo.
const _logoFolderFormat = "organizations/%s/logo"

// LogoPath is the URL path an organization's logo is served under. The
// router mounts the logo routes on this very path, and the value is stored
// on the organization row, so the two must not drift apart.
const LogoPath = "/api/organizations/logo"

// LogoFolder returns the storage folder holding the given organization's
// logo, which is stored under the organization's own id.
func LogoFolder(organizationID string) string {
	return fmt.Sprintf(_logoFolderFormat, organizationID)
}
