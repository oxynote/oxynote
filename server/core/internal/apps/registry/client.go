// Package registry fetches container image digests from container registries.
package registry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/go-containerregistry/pkg/authn"
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

	// ErrInvalidReference is returned when an image reference cannot be parsed.
	ErrInvalidReference = errutil.New(http.StatusBadRequest, "registry.invalid_reference", "invalid container image reference")
)

// digestOptions holds options for configuring the Digest function.
type digestOptions struct {
	username string
	password string
	token    string
}

// DigestOption is an option for configuring the Digest function.
type DigestOption func(*digestOptions)

// Digest retrieves the digest of the specified container image.
func Digest(
	ctx context.Context,
	image string,
	digOpts ...DigestOption,
) (string, error) {
	ref, err := parseReference(image)
	if err != nil {
		return "", err
	}

	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
	}

	baseOpts := &digestOptions{}

	for _, optFn := range digOpts {
		optFn(baseOpts)
	}

	if baseOpts.username != "" && baseOpts.password != "" {
		remoteOpts = append(remoteOpts,
			remote.WithAuth(
				&authn.Basic{
					Username: baseOpts.username,
					Password: baseOpts.password,
				},
			),
		)
	} else if baseOpts.token != "" {
		remoteOpts = append(remoteOpts,
			remote.WithAuth(
				&authn.Bearer{
					Token: baseOpts.token,
				},
			),
		)
	}

	desc, err := remote.Head(ref, remoteOpts...)
	if err != nil {
		if serr := statusError(err); serr != nil {
			return "", serr
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

// WithBasicAuth sets the username and password for basic authentication.
func WithBasicAuth(username, password string) DigestOption {
	return func(o *digestOptions) {
		o.username = username
		o.password = password
	}
}

// WithBearerToken sets the token for bearer authentication.
func WithBearerToken(token string) DigestOption {
	return func(o *digestOptions) {
		o.token = token
	}
}

// statusError maps a registry response the caller acts on to its error,
// or returns nil for any other failure.
func statusError(err error) error {
	terr, ok := errors.AsType[*transport.Error](err)
	if !ok {
		return nil
	}

	if terr.StatusCode == http.StatusUnauthorized ||
		terr.StatusCode == http.StatusForbidden ||
		slices.ContainsFunc(terr.Errors, func(diag transport.Diagnostic) bool {
			return diag.Code == transport.UnauthorizedErrorCode || diag.Code == transport.DeniedErrorCode
		}) {
		return ErrUnauthorized
	}

	// a HEAD response carries no body, so a missing image or tag shows
	// only as the status code.
	if terr.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}

	return nil
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
		return nil, ErrInvalidReference
	}

	return ref, nil
}
