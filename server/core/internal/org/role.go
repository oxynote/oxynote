package org

import (
	"net/http"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

const (
	// RoleAdmin represents a member who manages the organization, its
	// members, integrations and data sources, and edits everything else.
	RoleAdmin Role = "admin"

	// RoleEditor represents a member who edits documents, branches,
	// variables, tags and hooks.
	RoleEditor Role = "editor"

	// RoleViewer represents a member who reads, queries data sources and
	// changes only their own settings.
	RoleViewer Role = "viewer"
)

// ErrForbidden is returned when a member's role does not allow an
// action.
var ErrForbidden = errutil.New(http.StatusForbidden, "member.forbidden", "forbidden")

// _roleOrder lists the roles from least to most permitted.
var _roleOrder = []Role{RoleViewer, RoleEditor, RoleAdmin}

// Role represents a member's role in an organization. auth-realtime owns
// the list and stores it on the member.
type Role string

// Allows checks that the role may do what the given least role may do. An
// unknown role allows nothing, and nothing reaches an unknown least role.
func (r Role) Allows(least Role) error {
	rank := slices.Index(_roleOrder, r)
	leastRank := slices.Index(_roleOrder, least)

	if rank < 0 || leastRank < 0 || rank < leastRank {
		return ErrForbidden
	}

	return nil
}

// AtLeast reports whether the role may do what the given least role may
// do. It gates what a member is offered, where Allows refuses a request.
func (r Role) AtLeast(least Role) bool {
	return r.Allows(least) == nil
}

// CanEdit reports whether the role may change content.
func (r Role) CanEdit() bool {
	return r.AtLeast(RoleEditor)
}

// MarshalText returns the role as text.
func (r Role) MarshalText() ([]byte, error) {
	return []byte(r), nil
}

// UnmarshalText parses a role, trimmed and lowercased. An unknown role is
// kept, so Allows refuses it rather than the JSON failing to decode.
func (r *Role) UnmarshalText(text []byte) error {
	*r = Role(strings.ToLower(strings.TrimSpace(string(text))))

	return nil
}
