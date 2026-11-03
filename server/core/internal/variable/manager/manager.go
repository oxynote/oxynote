// Package manager runs the writes to an organization's global variables
// and announces them.
package manager

import (
	"context"
	"log/slog"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// Manager manages an organization's variable groups, variables, options
// and each user's own values.
type Manager struct {
	log            *slog.Logger
	db             DB
	changeCallback func(organizationID string, userID null.String)
}

// NewManager creates a new Manager with the given database interface.
func NewManager(log *slog.Logger, db DB) *Manager {
	return &Manager{
		log:            log.With("component", "variables-manager"),
		db:             db,
		changeCallback: func(string, null.String) {},
	}
}

// BindChange sets the function called after every write. A change of the
// organization's groups, variables, options or defaults comes with a null
// user; a change of one user's own value names that user. Call it once,
// before any write.
func (m *Manager) BindChange(fn func(organizationID string, userID null.String)) {
	m.changeCallback = fn
}

// FetchTree returns the organization's groups, variables and options in
// order, each variable with the value the user set.
func (m *Manager) FetchTree(ctx context.Context, organizationID, userID string) (variable.Tree, error) {
	return m.db.FetchVariableTree(ctx, organizationID, userID)
}

// CreateGroup creates a group at the end of the organization's groups.
func (m *Manager) CreateGroup(ctx context.Context, inp variable.GroupCreateInput, organizationID, userID string) (*variable.Group, error) {
	g, err := variable.NewGroup(inp, organizationID, userID)
	if err != nil {
		return nil, err
	}

	if err = m.db.CreateVariableGroup(ctx, g); err != nil {
		return nil, err
	}

	stored, err := m.db.FetchVariableGroup(ctx, g.ID, organizationID)
	if err != nil {
		return nil, err
	}

	m.changeCallback(organizationID, null.String{})

	return stored, nil
}

// UpdateGroup renames and/or recolours a group other than the built-in
// one.
func (m *Manager) UpdateGroup(ctx context.Context, id xid.ID, inp variable.GroupUpdateInput, organizationID string) error {
	g, err := m.db.FetchVariableGroup(ctx, id, organizationID)
	if err != nil {
		return err
	}

	if err = g.AllowsChange(); err != nil {
		return err
	}

	ng, err := g.ApplyUpdate(inp)
	if err != nil {
		return err
	}

	if err = m.db.UpdateVariableGroup(ctx, ng); err != nil {
		return err
	}

	m.changeCallback(organizationID, null.String{})

	return nil
}

// MoveGroup moves a group other than the built-in one to another
// position. The built-in group stays first, so the index counts only the
// other groups.
func (m *Manager) MoveGroup(ctx context.Context, inp variable.GroupMoveInput, organizationID string) error {
	groups, err := m.db.FetchVariableGroups(ctx, organizationID)
	if err != nil {
		return err
	}

	ids := make([]xid.ID, 0, len(groups))

	var target *variable.Group

	for _, g := range groups {
		if g.ID == inp.ID {
			target = &g
		}

		if !g.Builtin {
			ids = append(ids, g.ID)
		}
	}

	if target == nil {
		return errutil.ErrNotFound
	}

	if err = target.AllowsChange(); err != nil {
		return err
	}

	ids, err = variable.PlaceID(ids, inp.ID, inp.SortIndex)
	if err != nil {
		return err
	}

	if err = m.db.UpdateVariableGroupOrder(ctx, organizationID, ids); err != nil {
		return err
	}

	m.changeCallback(organizationID, null.String{})

	return nil
}

// DeleteGroup deletes an empty group other than the built-in one.
func (m *Manager) DeleteGroup(ctx context.Context, id xid.ID, organizationID string) error {
	g, err := m.db.FetchVariableGroup(ctx, id, organizationID)
	if err != nil {
		return err
	}

	if err = g.AllowsChange(); err != nil {
		return err
	}

	if err = m.db.DeleteVariableGroup(ctx, id, organizationID); err != nil {
		return err
	}

	m.changeCallback(organizationID, null.String{})

	return nil
}

// CreateVariable creates a variable at the end of a group other than the
// built-in one.
func (m *Manager) CreateVariable(ctx context.Context, inp variable.Input, organizationID, userID string) (*variable.Variable, error) {
	v, err := variable.NewVariable(inp, organizationID, userID)
	if err != nil {
		return nil, err
	}

	g, err := m.db.FetchVariableGroup(ctx, v.GroupID, organizationID)
	if err != nil {
		return nil, err
	}

	if err = g.AllowsChange(); err != nil {
		return nil, err
	}

	if err = m.db.CreateVariable(ctx, v); err != nil {
		return nil, err
	}

	stored, err := m.db.FetchVariable(ctx, v.ID, organizationID)
	if err != nil {
		return nil, err
	}

	m.changeCallback(organizationID, null.String{})

	return stored, nil
}

// UpdateVariable replaces the definition of a variable other than a
// built-in one. A type change clears every user's value; a removed option
// clears the values that picked it.
func (m *Manager) UpdateVariable(ctx context.Context, id xid.ID, inp variable.Input, organizationID string) error {
	v, err := m.db.FetchVariable(ctx, id, organizationID)
	if err != nil {
		return err
	}

	if err = v.AllowsChange(); err != nil {
		return err
	}

	nv, err := v.ApplyUpdate(inp)
	if err != nil {
		return err
	}

	if nv.GroupID != v.GroupID {
		g, gerr := m.db.FetchVariableGroup(ctx, nv.GroupID, organizationID)
		if gerr != nil {
			return gerr
		}

		if err = g.AllowsChange(); err != nil {
			return err
		}
	}

	var tx Tx

	if err = m.db.BeginTx(ctx, &tx); err != nil {
		return err
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err = tx.UpdateVariable(ctx, nv); err != nil {
		return err
	}

	if nv.Type != v.Type {
		if err = tx.DeleteVariableValues(ctx, organizationID, id); err != nil {
			return err
		}
	}

	if nv.Type == variable.TypeList || v.Type == variable.TypeList {
		if err = tx.ReplaceVariableOptions(ctx, nv); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	m.changeCallback(organizationID, null.String{})

	return nil
}

// MoveVariable moves a variable other than a built-in one to a position
// in its own group or in another group other than the built-in one.
func (m *Manager) MoveVariable(ctx context.Context, inp variable.MoveInput, organizationID string) error {
	v, err := m.db.FetchVariable(ctx, inp.ID, organizationID)
	if err != nil {
		return err
	}

	if err = v.AllowsChange(); err != nil {
		return err
	}

	g, err := m.db.FetchVariableGroup(ctx, inp.GroupID, organizationID)
	if err != nil {
		return err
	}

	if err = g.AllowsChange(); err != nil {
		return err
	}

	siblings, err := m.db.FetchVariablesByGroupID(ctx, inp.GroupID, organizationID)
	if err != nil {
		return err
	}

	ids := make([]xid.ID, 0, len(siblings)+1)

	for _, s := range siblings {
		ids = append(ids, s.ID)
	}

	ids, err = variable.PlaceID(ids, inp.ID, inp.SortIndex)
	if err != nil {
		return err
	}

	if err = m.db.UpdateVariableOrder(ctx, organizationID, inp.GroupID, ids); err != nil {
		return err
	}

	m.changeCallback(organizationID, null.String{})

	return nil
}

// DeleteVariable deletes a variable other than a built-in one, with its
// options and every user's value.
func (m *Manager) DeleteVariable(ctx context.Context, id xid.ID, organizationID string) error {
	v, err := m.db.FetchVariable(ctx, id, organizationID)
	if err != nil {
		return err
	}

	if err = v.AllowsChange(); err != nil {
		return err
	}

	if err = m.db.DeleteVariable(ctx, id, organizationID); err != nil {
		return err
	}

	m.changeCallback(organizationID, null.String{})

	return nil
}

// SetDefaultValue sets the value of a variable every user without their
// own value sees. It is the one change a built-in variable allows.
func (m *Manager) SetDefaultValue(ctx context.Context, id xid.ID, value, organizationID string) error {
	v, err := m.db.FetchVariable(ctx, id, organizationID)
	if err != nil {
		return err
	}

	nv, err := v.ApplyDefault(value)
	if err != nil {
		return err
	}

	if err = m.db.UpdateVariableDefault(ctx, nv); err != nil {
		return err
	}

	m.changeCallback(organizationID, null.String{})

	return nil
}

// SetValue sets the user's own value of a variable, leaving every other
// user's value as it is.
func (m *Manager) SetValue(ctx context.Context, id xid.ID, value, organizationID, userID string) error {
	v, err := m.db.FetchVariable(ctx, id, organizationID)
	if err != nil {
		return err
	}

	if err = v.ValidateValue(value); err != nil {
		return err
	}

	if err = m.db.UpsertVariableValue(ctx, organizationID, userID, id, value); err != nil {
		return err
	}

	m.changeCallback(organizationID, null.StringFrom(userID))

	return nil
}

// ResetValue removes the user's own value of a variable, so they see the
// default again.
func (m *Manager) ResetValue(ctx context.Context, id xid.ID, organizationID, userID string) error {
	if _, err := m.db.FetchVariable(ctx, id, organizationID); err != nil {
		return err
	}

	if err := m.db.DeleteVariableValue(ctx, organizationID, userID, id); err != nil {
		return err
	}

	m.changeCallback(organizationID, null.StringFrom(userID))

	return nil
}

// DBAgent defines the queries the Manager runs, inside a transaction or
// not.
//
//go:generate ../../../scripts/codegen/mock -t internal DBAgent db_agent
type DBAgent interface {
	GroupDBAgent
	OptionDBAgent

	// CreateVariable should insert the variable at the end of its group,
	// with its options.
	CreateVariable(ctx context.Context, v variable.Variable) error

	// FetchVariable should fetch one variable of the organization with
	// its options.
	FetchVariable(ctx context.Context, id xid.ID, organizationID string) (*variable.Variable, error)

	// FetchVariablesByGroupID should fetch the variables of one group in
	// display order.
	FetchVariablesByGroupID(ctx context.Context, groupID xid.ID, organizationID string) ([]variable.Variable, error)

	// FetchVariableTree should fetch the organization's groups, variables
	// and options in order, each variable with the user's value.
	FetchVariableTree(ctx context.Context, organizationID, userID string) (variable.Tree, error)

	// UpdateVariable should rewrite the variable's definition.
	UpdateVariable(ctx context.Context, v variable.Variable) error

	// UpdateVariableDefault should rewrite the variable's default and its
	// options' order, refusing a variable whose type changed.
	UpdateVariableDefault(ctx context.Context, v variable.Variable) error

	// UpdateVariableOrder should put the variables in the group, in the
	// order of the list.
	UpdateVariableOrder(ctx context.Context, organizationID string, groupID xid.ID, ids []xid.ID) error

	// DeleteVariable should delete the variable with its options and
	// values.
	DeleteVariable(ctx context.Context, id xid.ID, organizationID string) error
}

// GroupDBAgent defines the queries the Manager runs on groups.
type GroupDBAgent interface {
	// CreateVariableGroup should insert the group at the end of its
	// organization's groups.
	CreateVariableGroup(ctx context.Context, g variable.Group) error

	// FetchVariableGroup should fetch one group of the organization.
	FetchVariableGroup(ctx context.Context, id xid.ID, organizationID string) (*variable.Group, error)

	// FetchVariableGroups should fetch the organization's groups in
	// display order, the built-in one first.
	FetchVariableGroups(ctx context.Context, organizationID string) ([]variable.Group, error)

	// UpdateVariableGroup should rewrite the group's name and colour.
	UpdateVariableGroup(ctx context.Context, g variable.Group) error

	// UpdateVariableGroupOrder should rewrite the groups' display order to
	// their order in the list.
	UpdateVariableGroupOrder(ctx context.Context, organizationID string, ids []xid.ID) error

	// DeleteVariableGroup should delete the group.
	DeleteVariableGroup(ctx context.Context, id xid.ID, organizationID string) error
}

// OptionDBAgent defines the queries the Manager runs on a variable's
// options and each user's value.
type OptionDBAgent interface {
	// ReplaceVariableOptions should make the variable's options the only
	// ones it has, dropping the user values of removed ones.
	ReplaceVariableOptions(ctx context.Context, v variable.Variable) error

	// UpsertVariableValue should record the user's own value.
	UpsertVariableValue(ctx context.Context, organizationID, userID string, variableID xid.ID, value string) error

	// DeleteVariableValue should delete the user's own value.
	DeleteVariableValue(ctx context.Context, organizationID, userID string, variableID xid.ID) error

	// DeleteVariableValues should delete every user's value of the
	// variable.
	DeleteVariableValues(ctx context.Context, organizationID string, variableID xid.ID) error
}

// DB defines the database the Manager runs on.
//
//go:generate ../../../scripts/codegen/mock -t internal DB db
type DB interface {
	sqlutil.DB
	DBAgent
}

// Tx defines the transaction a multi-statement write runs in.
//
//go:generate ../../../scripts/codegen/mock -t internal Tx tx
type Tx interface {
	sqlutil.Tx
	DBAgent
}
