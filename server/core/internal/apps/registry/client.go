// Package registry fetches container image digests from container registries.
package registry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

var (
	// ErrUnauthorized is returned when access to the container registry is unauthorized.
	ErrUnauthorized = errutil.New(http.StatusBadRequest, "registry.unauthorized", "unauthorized access to the container registry")

	// ErrNotFound is returned when the registry has no such image or tag.
	ErrNotFound = errutil.New(http.StatusNotFound, "registry.not_found", "container image not found")
)

// Digest retrieves the digest of the specified container image.
func Digest(ctx context.Context, image string) (string, error) {
	ref, err := parseReference(image)
	if err != nil {
		return "", err
	}

	desc, err := remote.Head(ref, remote.WithContext(ctx))
	if err != nil {
		if terr, ok := errors.AsType[*transport.Error](err); ok {
			switch {
			case terr.StatusCode == http.StatusUnauthorized,
				terr.StatusCode == http.StatusForbidden,
				slices.ContainsFunc(terr.Errors, func(diag transport.Diagnostic) bool {
					return diag.Code == transport.UnauthorizedErrorCode || diag.Code == transport.DeniedErrorCode
				}):
				return "", ErrUnauthorized
			// a HEAD response carries no body, so a missing image or tag
			// shows only as the status code.
			case terr.StatusCode == http.StatusNotFound:
				return "", ErrNotFound
			}
		}

		return "", fmt.Errorf("getting remote head for %q: %w", ref.Name(), err)
	}

	return desc.Digest.String(), nil
}

// ValidateReference checks that the image reference can be parsed.
func ValidateReference(image string) error {
	_, err := parseReference(image)

	return err
}

// parseReference parses an image reference, defaulting to Docker Hub and the
// latest tag.
func parseReference(image string) (name.Reference, error) {
	ref, err := name.ParseReference(
		image,
		name.WithDefaultRegistry("index.docker.io"),
		name.WithDefaultTag("latest"),
	)
	if err != nil {
		return nil, fmt.Errorf("parsing image reference %q: %w", image, err)
	}

	return ref, nil
}
