package errutil

import (
	"github.com/hashicorp/go-multierror"
	"github.com/jellydator/validation"
	"golang.org/x/exp/maps"
)

// Combine combines the provided errors into one.
// If both errors are validation.Errors, then they are
// combined in validation.Errors.
// Otherwise, a new multi error is created.
func Combine(errs ...error) error {
	var verr validation.Errors

	for i := range errs {
		if errs[i] == nil {
			continue
		}

		verr1, ok := errs[i].(validation.Errors) //nolint:errorlint // we need a direct check
		if !ok {
			verr = nil
			break
		}

		if verr == nil {
			verr = maps.Clone(verr1)
			continue
		}

		maps.Copy(verr, verr1)
	}

	if verr == nil {
		return multierror.Append(nil, errs...).ErrorOrNil()
	}

	return verr
}
