package org

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Role_Allows(t *testing.T) {
	cc := map[string]struct {
		Role  Role
		Least Role
		Err   error
	}{
		"Admin allows admin":             {Role: RoleAdmin, Least: RoleAdmin},
		"Admin allows editor":            {Role: RoleAdmin, Least: RoleEditor},
		"Admin allows viewer":            {Role: RoleAdmin, Least: RoleViewer},
		"Editor refuses admin":           {Role: RoleEditor, Least: RoleAdmin, Err: ErrForbidden},
		"Editor allows editor":           {Role: RoleEditor, Least: RoleEditor},
		"Editor allows viewer":           {Role: RoleEditor, Least: RoleViewer},
		"Viewer refuses admin":           {Role: RoleViewer, Least: RoleAdmin, Err: ErrForbidden},
		"Viewer refuses editor":          {Role: RoleViewer, Least: RoleEditor, Err: ErrForbidden},
		"Viewer allows viewer":           {Role: RoleViewer, Least: RoleViewer},
		"Unknown role refuses all":       {Role: "owner", Least: RoleViewer, Err: ErrForbidden},
		"Empty role refuses all":         {Role: "", Least: RoleViewer, Err: ErrForbidden},
		"Unknown least role refuses all": {Role: RoleAdmin, Least: "owner", Err: ErrForbidden},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Role.Allows(c.Least))
		})
	}
}

func Test_Role_AtLeast(t *testing.T) {
	cc := map[string]struct {
		Role   Role
		Least  Role
		Result bool
	}{
		"Viewer reaches viewer":   {Role: RoleViewer, Least: RoleViewer, Result: true},
		"Viewer misses editor":    {Role: RoleViewer, Least: RoleEditor},
		"Admin reaches editor":    {Role: RoleAdmin, Least: RoleEditor, Result: true},
		"Unknown role misses all": {Role: "owner", Least: RoleViewer},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Role.AtLeast(c.Least))
		})
	}
}

func Test_Role_CanEdit(t *testing.T) {
	cc := map[string]struct {
		Role   Role
		Result bool
	}{
		"Admin can edit":           {Role: RoleAdmin, Result: true},
		"Editor can edit":          {Role: RoleEditor, Result: true},
		"Viewer cannot edit":       {Role: RoleViewer},
		"Unknown role cannot edit": {Role: "owner"},
		"Empty role cannot edit":   {Role: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Role.CanEdit())
		})
	}
}

func Test_Role_MarshalText(t *testing.T) {
	t.Parallel()

	res, err := RoleEditor.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, []byte("editor"), res)
}

func Test_Role_UnmarshalText(t *testing.T) {
	cc := map[string]struct {
		Text   string
		Result Role
	}{
		"Known role":             {Text: "viewer", Result: RoleViewer},
		"Trimmed and lowercased": {Text: " Admin ", Result: RoleAdmin},
		"Unknown role is kept":   {Text: "owner", Result: "owner"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var res Role

			require.NoError(t, res.UnmarshalText([]byte(c.Text)))
			assert.Equal(t, c.Result, res)
		})
	}
}
