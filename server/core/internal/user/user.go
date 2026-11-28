// Package user defines what belongs to a user account: the storage layout
// of their image, the URL the image is served under, and the state of the
// spotlights the app shows them once.
package user

import "fmt"

// ImageFolder is the storage folder holding every user's image, each under
// its user id. It is keyed by user only: the avatar lives on the global
// user row, so an org-scoped object would 404 for viewers with a different
// active organization.
const ImageFolder = "users/images"

// ImagePathFormat is the URL path format a user's image is served under,
// taking the user id. The router mounts the image routes on this very
// format, and the value is stored on the user row, so the two must not
// drift apart.
const ImagePathFormat = "/api/users/%s/image"

// ImagePath returns the URL path the given user's image is served under.
func ImagePath(userID string) string {
	return fmt.Sprintf(ImagePathFormat, userID)
}
