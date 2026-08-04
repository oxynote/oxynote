package contregistry

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

// ErrUnauthorized is returned when access to the container registry is unauthorized.
var ErrUnauthorized = errutil.New(http.StatusBadRequest, "contregistry.unauthorized", "Unauthorized access to the container registry.")

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
	ref, err := name.ParseReference(
		image,
		name.WithDefaultRegistry("index.docker.io"),
		name.WithDefaultTag("latest"),
	)
	if err != nil {
		return "", fmt.Errorf("parsing image reference %q: %w", image, err)
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
		var terr *transport.Error

		if ok := errors.As(err, &terr); ok {
			if slices.ContainsFunc(terr.Errors, func(diag transport.Diagnostic) bool {
				return diag.Code == transport.UnauthorizedErrorCode || diag.Code == transport.DeniedErrorCode
			}) {
				return "", ErrUnauthorized
			}
		}

		return "", fmt.Errorf("getting remote head for %q: %w", ref.Name(), err)
	}

	return desc.Digest.String(), nil
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
