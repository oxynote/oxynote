package db

import (
	"context"
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// CreateVariableGroup inserts a new group at the end of its organization's
// groups. The position is one past the highest index, so gaps left by
// deletions are skipped over.
func (a *agent) CreateVariableGroup(ctx context.Context, g variable.Group) error {
	// the inner select keeps "?" placeholders, which the insert numbers.
	sel := sq.Select().
		Column("?", g.ID).
		Column("?", g.OrganizationID).
		Column("?", g.GroupName).
		Column("?", g.Color).
		Column("COALESCE(MAX(sort_index) + 1, 0)").
		Column("?", g.Builtin).
		Column("?", g.CreatedAt).
		Column("?", g.CreatedBy).
		From("personal_variable_groups").
		Where(sq.Eq{"fk_organization_id": g.OrganizationID})

	q, args := a.builder.Insert("personal_variable_groups").
		Columns(
			"id",
			"fk_organization_id",
			"group_name",
			"color",
			"sort_index",
			"builtin",
			"created_at",
			"fk_created_by",
		).
		Select(sel).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// CreateVariable inserts a new variable at the end of its group, together
// with its options.
func (a *agent) CreateVariable(ctx context.Context, v variable.Variable) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		// the inner select keeps "?" placeholders, which the insert numbers.
		sel := sq.Select().
			Column("?", v.ID).
			Column("?", v.OrganizationID).
			Column("?", v.GroupID).
			Column("?", v.Key).
			Column("?", v.Name).
			Column("?", v.Description).
			Column("?", v.Type).
			Column("?", v.DefaultValue).
			Column("COALESCE(MAX(sort_index) + 1, 0)").
			Column("?", v.Builtin).
			Column("?", v.CreatedAt).
			Column("?", v.CreatedBy).
			From("personal_variables").
			Where(sq.Eq{"fk_variable_group_id": v.GroupID})

		q, args := a.builder.Insert("personal_variables").
			Columns(
				"id",
				"fk_organization_id",
				"fk_variable_group_id",
				"variable_key",
				"variable_name",
				"description",
				"variable_type",
				"default_value",
				"sort_index",
				"builtin",
				"created_at",
				"fk_created_by",
			).
			Select(sel).
			MustSql()

		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}

		return a.upsertVariableOptions(ctx, tx, v.Options)
	})
}

// FetchVariableGroup retrieves one group of the organization.
func (a *agent) FetchVariableGroup(ctx context.Context, id xid.ID, organizationID string) (*variable.Group, error) {
	q, args := a.selectVariableGroup(a.builder.Select(), organizationID).
		Where(sq.Eq{"id": id}).
		MustSql()

	var g variable.Group

	if err := sqlx.GetContext(ctx, a.sql, &g, q, args...); err != nil {
		return nil, err
	}

	return &g, nil
}

// FetchVariableGroups retrieves the organization's groups in display
// order, the built-in one first.
func (a *agent) FetchVariableGroups(ctx context.Context, organizationID string) ([]variable.Group, error) {
	q, args := a.selectVariableGroup(a.builder.Select(), organizationID).MustSql()

	groups := []variable.Group{}

	if err := sqlx.SelectContext(ctx, a.sql, &groups, q, args...); err != nil {
		return nil, err
	}

	return groups, nil
}

// FetchVariable retrieves one variable of the organization with its
// options.
func (a *agent) FetchVariable(ctx context.Context, id xid.ID, organizationID string) (*variable.Variable, error) {
	q, args := a.selectVariable(a.builder.Select(), organizationID).
		Where(sq.Eq{"personal_variables.id": id}).
		MustSql()

	var v variable.Variable

	if err := sqlx.GetContext(ctx, a.sql, &v, q, args...); err != nil {
		return nil, err
	}

	options, err := a.fetchVariableOptions(ctx, organizationID, []xid.ID{v.ID})
	if err != nil {
		return nil, err
	}

	v.Options = options[v.ID]

	return &v, nil
}

// FetchVariablesByGroupID retrieves the variables of one group in display
// order, each with its options.
func (a *agent) FetchVariablesByGroupID(ctx context.Context, groupID xid.ID, organizationID string) ([]variable.Variable, error) {
	q, args := a.selectVariable(a.builder.Select(), organizationID).
		Where(sq.Eq{"personal_variables.fk_variable_group_id": groupID}).
		MustSql()

	vars := []variable.Variable{}

	if err := sqlx.SelectContext(ctx, a.sql, &vars, q, args...); err != nil {
		return nil, err
	}

	ids := make([]xid.ID, 0, len(vars))

	for _, v := range vars {
		ids = append(ids, v.ID)
	}

	options, err := a.fetchVariableOptions(ctx, organizationID, ids)
	if err != nil {
		return nil, err
	}

	for i, v := range vars {
		vars[i].Options = options[v.ID]
	}

	return vars, nil
}

// FetchVariableTree retrieves the organization's groups, variables and
// options in display order, each variable with the value the given user
// set.
func (a *agent) FetchVariableTree(ctx context.Context, organizationID, userID string) (variable.Tree, error) {
	groups, err := a.FetchVariableGroups(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	q, args := a.selectVariable(a.builder.Select(), organizationID).
		Column(`vv.value AS "value"`).
		LeftJoin("personal_variable_values vv ON vv.fk_variable_id = personal_variables.id AND vv.fk_user_id = ?", userID).
		MustSql()

	entries := []variable.Entry{}

	if err = sqlx.SelectContext(ctx, a.sql, &entries, q, args...); err != nil {
		return nil, err
	}

	ids := make([]xid.ID, 0, len(entries))

	for _, e := range entries {
		ids = append(ids, e.ID)
	}

	options, err := a.fetchVariableOptions(ctx, organizationID, ids)
	if err != nil {
		return nil, err
	}

	for i, e := range entries {
		entries[i].Options = options[e.ID]
	}

	return variable.NewTree(groups, entries), nil
}

// UpdateVariableGroup rewrites a group's name and colour. A group outside
// the organization is not found.
func (a *agent) UpdateVariableGroup(ctx context.Context, g variable.Group) error {
	q, args := a.builder.Update("personal_variable_groups").
		SetMap(map[string]any{
			"group_name": g.GroupName,
			"color":      g.Color,
		}).
		Where(sq.Eq{
			"id":                 g.ID,
			"fk_organization_id": g.OrganizationID,
		}).
		MustSql()

	res, err := a.sql.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return errutil.ErrNotFound
	}

	return nil
}

// UpdateVariableGroupOrder rewrites the display order of the given groups
// to their order in the list. A group outside the organization aborts the
// whole rewrite.
func (a *agent) UpdateVariableGroupOrder(ctx context.Context, organizationID string, ids []xid.ID) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		for i, id := range ids {
			q, args := a.builder.Update("personal_variable_groups").
				Set("sort_index", i).
				Where(sq.Eq{
					"id":                 id,
					"fk_organization_id": organizationID,
				}).
				MustSql()

			res, err := tx.ExecContext(ctx, q, args...)
			if err != nil {
				return err
			}

			n, err := res.RowsAffected()
			if err != nil {
				return err
			}

			if n == 0 {
				return errutil.ErrNotFound
			}
		}

		return nil
	})
}

// UpdateVariable rewrites a variable's definition. A variable that changes
// group goes to the end of the new one. A variable outside the
// organization is not found.
func (a *agent) UpdateVariable(ctx context.Context, v variable.Variable) error {
	// every expression in a SET reads the row as it was, so the CASE
	// compares against the old group.
	q, args := a.builder.Update("personal_variables").
		Set("sort_index", sq.Expr(
			"CASE WHEN fk_variable_group_id = ? THEN sort_index "+
				"ELSE (SELECT COALESCE(MAX(sort_index) + 1, 0) FROM personal_variables WHERE fk_variable_group_id = ?) END",
			v.GroupID,
			v.GroupID,
		)).
		SetMap(map[string]any{
			"fk_variable_group_id": v.GroupID,
			"variable_key":         v.Key,
			"variable_name":        v.Name,
			"description":          v.Description,
			"variable_type":        v.Type,
			"default_value":        v.DefaultValue,
		}).
		Where(sq.Eq{
			"id":                 v.ID,
			"fk_organization_id": v.OrganizationID,
		}).
		MustSql()

	res, err := a.sql.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return errutil.ErrNotFound
	}

	return nil
}

// UpdateVariableDefault rewrites only a variable's default value and, for
// a list, its options' order. A variable whose type changed, or a list
// without the chosen option, is not found, so a stale default is never
// stored.
func (a *agent) UpdateVariableDefault(ctx context.Context, v variable.Variable) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		b := a.builder.Update("personal_variables").
			Set("default_value", v.DefaultValue).
			Where(sq.Eq{
				"id":                 v.ID,
				"fk_organization_id": v.OrganizationID,
				"variable_type":      v.Type,
			})

		if v.Type == variable.TypeList {
			b = b.Where(
				"EXISTS (SELECT 1 FROM personal_variable_options WHERE id = ? AND fk_variable_id = ?)",
				v.DefaultValue,
				v.ID,
			)
		}

		q, args := b.MustSql()

		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return err
		}

		n, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if n == 0 {
			return errutil.ErrNotFound
		}

		// another option removed since the variable was read updates
		// nothing, which only leaves a gap in the order.
		for _, o := range v.Options {
			q, args = a.builder.Update("personal_variable_options").
				Set("sort_index", o.SortIndex).
				Where(sq.Eq{
					"id":             o.ID,
					"fk_variable_id": v.ID,
				}).
				MustSql()

			if _, err = tx.ExecContext(ctx, q, args...); err != nil {
				return err
			}
		}

		return nil
	})
}

// UpdateVariableOrder puts the given variables in the group, in the order
// of the list. A variable outside the organization aborts the whole
// rewrite.
func (a *agent) UpdateVariableOrder(ctx context.Context, organizationID string, groupID xid.ID, ids []xid.ID) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		for i, id := range ids {
			q, args := a.builder.Update("personal_variables").
				SetMap(map[string]any{
					"fk_variable_group_id": groupID,
					"sort_index":           i,
				}).
				Where(sq.Eq{
					"id":                 id,
					"fk_organization_id": organizationID,
				}).
				MustSql()

			res, err := tx.ExecContext(ctx, q, args...)
			if err != nil {
				return err
			}

			n, err := res.RowsAffected()
			if err != nil {
				return err
			}

			if n == 0 {
				return errutil.ErrNotFound
			}
		}

		return nil
	})
}

// ReplaceVariableOptions makes the variable's options, in order, the only
// ones it has. Every user value that points at a removed option is
// removed with it, so those users see the default again.
func (a *agent) ReplaceVariableOptions(ctx context.Context, v variable.Variable) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		var exists int

		eq, eargs := a.builder.Select("1").
			From("personal_variables").
			Where(sq.Eq{
				"id":                 v.ID,
				"fk_organization_id": v.OrganizationID,
			}).
			MustSql()

		if err := sqlx.GetContext(ctx, tx, &exists, eq, eargs...); err != nil {
			return err
		}

		kept := make([]string, 0, len(v.Options))

		for _, o := range v.Options {
			kept = append(kept, o.ID.String())
		}

		q, args := a.builder.Delete("personal_variable_options").
			Where(sq.Eq{"fk_variable_id": v.ID}).
			Where(sq.NotEq{"id": kept}).
			MustSql()

		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}

		q, args = a.builder.Delete("personal_variable_values").
			Where(sq.Eq{"fk_variable_id": v.ID}).
			Where(sq.NotEq{"value": kept}).
			MustSql()

		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}

		return a.upsertVariableOptions(ctx, tx, v.Options)
	})
}

// UpsertVariableValue records the value one user set for a variable,
// leaving every other user's value untouched.
func (a *agent) UpsertVariableValue(ctx context.Context, organizationID, userID string, variableID xid.ID, value string) error {
	// selecting the row out of the variable makes a variable outside the
	// organization insert nothing. The inner select keeps "?" placeholders,
	// which the insert numbers.
	sel := sq.Select("id").
		Column("?", userID).
		Column("fk_organization_id").
		Column("?", value).
		Column("?", timeutil.Now()).
		From("personal_variables").
		Where(sq.Eq{
			"id":                 variableID,
			"fk_organization_id": organizationID,
		})

	q, args := a.builder.Insert("personal_variable_values").
		Columns(
			"fk_variable_id",
			"fk_user_id",
			"fk_organization_id",
			"value",
			"updated_at",
		).
		Select(sel).
		Suffix("ON CONFLICT (fk_variable_id, fk_user_id) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at").
		MustSql()

	res, err := a.sql.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return errutil.ErrNotFound
	}

	return nil
}

// DeleteVariableGroup removes a group. A group that still holds variables
// is refused.
func (a *agent) DeleteVariableGroup(ctx context.Context, id xid.ID, organizationID string) error {
	q, args := a.builder.Delete("personal_variable_groups").
		Where(sq.Eq{
			"id":                 id,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	res, err := a.sql.ExecContext(ctx, q, args...)
	if err != nil {
		// a variable still in the group blocks the delete.
		if perr, ok := errors.AsType[*pgconn.PgError](err); ok && perr.Code == "23503" {
			return variable.ErrGroupNotEmpty
		}

		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return errutil.ErrNotFound
	}

	return nil
}

// DeleteVariable removes a variable; its options and every user's value
// go with it through the cascades.
func (a *agent) DeleteVariable(ctx context.Context, id xid.ID, organizationID string) error {
	q, args := a.builder.Delete("personal_variables").
		Where(sq.Eq{
			"id":                 id,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	res, err := a.sql.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return errutil.ErrNotFound
	}

	return nil
}

// DeleteVariableValue removes the value one user set for a variable, so
// they see the default again. Removing a value never set changes nothing.
func (a *agent) DeleteVariableValue(ctx context.Context, organizationID, userID string, variableID xid.ID) error {
	q, args := a.builder.Delete("personal_variable_values").
		Where(sq.Eq{
			"fk_variable_id":     variableID,
			"fk_user_id":         userID,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// DeleteVariableValues removes every user's value for a variable.
func (a *agent) DeleteVariableValues(ctx context.Context, organizationID string, variableID xid.ID) error {
	q, args := a.builder.Delete("personal_variable_values").
		Where(sq.Eq{
			"fk_variable_id":     variableID,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// upsertVariableOptions inserts the options or rewrites the ones that
// exist. An id held by another variable's option is left alone and the
// write is refused.
func (a *agent) upsertVariableOptions(ctx context.Context, ex sqlx.ExecerContext, options []variable.Option) error {
	// the builder refuses an insert without rows, and there is nothing to
	// write anyway.
	if len(options) == 0 {
		return nil
	}

	b := a.builder.Insert("personal_variable_options").
		Columns(
			"id",
			"fk_variable_id",
			"option_value",
			"sort_index",
		).
		Suffix("ON CONFLICT (id) DO UPDATE SET option_value = EXCLUDED.option_value, sort_index = EXCLUDED.sort_index " +
			"WHERE personal_variable_options.fk_variable_id = EXCLUDED.fk_variable_id")

	for _, o := range options {
		b = b.Values(o.ID, o.VariableID, o.Value, o.SortIndex)
	}

	q, args := b.MustSql()

	res, err := ex.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n < int64(len(options)) {
		return variable.ErrUnknownOption
	}

	return nil
}

// fetchVariableOptions retrieves the options of the given variables of
// the organization in order, keyed by variable. A variable without
// options maps to an empty slice.
func (a *agent) fetchVariableOptions(ctx context.Context, organizationID string, variableIDs []xid.ID) (map[xid.ID][]variable.Option, error) {
	q, args := a.selectVariableOption(a.builder.Select(), organizationID).
		Where(sq.Eq{"o.fk_variable_id": variableIDs}).
		MustSql()

	var options []variable.Option

	if err := sqlx.SelectContext(ctx, a.sql, &options, q, args...); err != nil {
		return nil, err
	}

	res := make(map[xid.ID][]variable.Option, len(variableIDs))

	for _, id := range variableIDs {
		res[id] = []variable.Option{}
	}

	for _, o := range options {
		res[o.VariableID] = append(res[o.VariableID], o)
	}

	return res, nil
}

// selectVariableGroup prepares a select statement for groups in display
// order, the built-in one first, scoped to the organization.
func (a *agent) selectVariableGroup(b sq.SelectBuilder, organizationID string) sq.SelectBuilder {
	return b.Columns(
		"id",
		"fk_organization_id",
		"group_name",
		"color",
		"sort_index",
		"builtin",
		"created_at",
		"fk_created_by",
	).From("personal_variable_groups").
		Where(sq.Eq{"fk_organization_id": organizationID}).
		OrderBy("builtin DESC", "sort_index", "id")
}

// selectVariable prepares a select statement for variables in display
// order within their group, scoped to the organization.
func (a *agent) selectVariable(b sq.SelectBuilder, organizationID string) sq.SelectBuilder {
	return b.Columns(
		"personal_variables.id",
		"personal_variables.fk_organization_id",
		"personal_variables.fk_variable_group_id",
		"personal_variables.variable_key",
		"personal_variables.variable_name",
		"personal_variables.description",
		"personal_variables.variable_type",
		"personal_variables.default_value",
		"personal_variables.sort_index",
		"personal_variables.builtin",
		"personal_variables.created_at",
		"personal_variables.fk_created_by",
	).From("personal_variables").
		Where(sq.Eq{"personal_variables.fk_organization_id": organizationID}).
		OrderBy("personal_variables.sort_index", "personal_variables.id")
}

// selectVariableOption prepares a select statement for options in order
// within their variable, scoped to the organization through it.
func (a *agent) selectVariableOption(b sq.SelectBuilder, organizationID string) sq.SelectBuilder {
	return b.Columns(
		"o.id",
		"o.fk_variable_id",
		"o.option_value",
		"o.sort_index",
	).From("personal_variable_options o").
		Join("personal_variables v ON v.id = o.fk_variable_id").
		Where(sq.Eq{"v.fk_organization_id": organizationID}).
		OrderBy("o.sort_index", "o.id")
}
