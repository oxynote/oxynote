package db

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepVariableGroups(t *testing.T, db *DB, count int, fn func(int, *variable.Group)) []variable.Group {
	t.Helper()

	res := make([]variable.Group, count)

	now := timeutil.Now().Truncate(time.Second)

	// groups missing an organization after fn ran share one lazily
	// created organization, so a single call produces sibling groups.
	var organizationID string

	for i := range count {
		g := variable.Group{
			ID:        xid.New(),
			GroupName: "Group " + strconv.Itoa(i) + " " + xid.New().String(),
			Color:     "green",
			SortIndex: i,
			CreatedAt: now,
		}

		if fn != nil {
			fn(i, &g)
		}

		if g.OrganizationID == "" {
			if organizationID == "" {
				organizationID = prepOrganizations(t, db, 1)[0]
			}

			g.OrganizationID = organizationID
		}

		res[i] = g

		q, args := db.builder.Insert("personal_variable_groups").
			SetMap(map[string]any{
				"id":                 g.ID,
				"fk_organization_id": g.OrganizationID,
				"group_name":         g.GroupName,
				"color":              g.Color,
				"sort_index":         g.SortIndex,
				"builtin":            g.Builtin,
				"created_at":         g.CreatedAt,
				"fk_created_by":      g.CreatedBy,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func prepVariables(t *testing.T, db *DB, count int, fn func(int, *variable.Variable)) []variable.Variable {
	t.Helper()

	res := make([]variable.Variable, count)

	now := timeutil.Now().Truncate(time.Second)

	// variables missing a group after fn ran share one lazily created
	// group, so a single call produces siblings.
	var group *variable.Group

	for i := range count {
		v := variable.Variable{
			ID:           xid.New(),
			Key:          "key_" + strconv.Itoa(i) + "_" + xid.New().String(),
			Name:         "Variable " + strconv.Itoa(i),
			Type:         variable.TypeText,
			DefaultValue: null.StringFrom("value " + strconv.Itoa(i)),
			SortIndex:    i,
			CreatedAt:    now,
			Options:      []variable.Option{},
		}

		if fn != nil {
			fn(i, &v)
		}

		if v.GroupID.IsZero() {
			if group == nil {
				group = &prepVariableGroups(t, db, 1, func(_ int, g *variable.Group) {
					g.OrganizationID = v.OrganizationID
				})[0]
			}

			v.GroupID = group.ID
			v.OrganizationID = group.OrganizationID
		}

		res[i] = v

		q, args := db.builder.Insert("personal_variables").
			SetMap(map[string]any{
				"id":                   v.ID,
				"fk_organization_id":   v.OrganizationID,
				"fk_variable_group_id": v.GroupID,
				"variable_key":         v.Key,
				"variable_name":        v.Name,
				"description":          v.Description,
				"variable_type":        v.Type,
				"default_value":        v.DefaultValue,
				"sort_index":           v.SortIndex,
				"builtin":              v.Builtin,
				"created_at":           v.CreatedAt,
				"fk_created_by":        v.CreatedBy,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func prepVariableOptions(t *testing.T, db *DB, variableID xid.ID, values ...string) []variable.Option {
	t.Helper()

	res := make([]variable.Option, len(values))

	for i, value := range values {
		o := variable.Option{
			ID:         xid.New(),
			VariableID: variableID,
			Value:      value,
			SortIndex:  i,
		}

		res[i] = o

		q, args := db.builder.Insert("personal_variable_options").
			SetMap(map[string]any{
				"id":             o.ID,
				"fk_variable_id": o.VariableID,
				"option_value":   o.Value,
				"sort_index":     o.SortIndex,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func prepVariableValue(t *testing.T, db *DB, v variable.Variable, userID, value string) {
	t.Helper()

	q, args := db.builder.Insert("personal_variable_values").
		SetMap(map[string]any{
			"fk_variable_id":     v.ID,
			"fk_user_id":         userID,
			"fk_organization_id": v.OrganizationID,
			"value":              value,
			"updated_at":         timeutil.Now(),
		}).MustSql()

	_, err := db.sql.Exec(q, args...)
	require.NoError(t, err)
}

func storedVariableValues(t *testing.T, db *DB, variableID xid.ID) map[string]string {
	t.Helper()

	q, args := db.builder.Select("fk_user_id", "value").
		From("personal_variable_values").
		Where(sq.Eq{"fk_variable_id": variableID}).
		MustSql()

	var rows []struct {
		UserID string `db:"fk_user_id"`
		Value  string `db:"value"`
	}

	require.NoError(t, sqlx.Select(db.sql, &rows, q, args...))

	res := map[string]string{}

	for _, r := range rows {
		res[r.UserID] = r.Value
	}

	return res
}

func storedVariableOptions(t *testing.T, db *DB, variableID xid.ID) []variable.Option {
	t.Helper()

	q, args := db.builder.Select("id", "fk_variable_id", "option_value", "sort_index").
		From("personal_variable_options").
		Where(sq.Eq{"fk_variable_id": variableID}).
		OrderBy("sort_index", "id").
		MustSql()

	options := []variable.Option{}

	require.NoError(t, sqlx.Select(db.sql, &options, q, args...))

	return options
}

func Test_agent_CreateVariableGroup(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Group            variable.Group
		SortIndex        int
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				Group:            variable.Group{ID: xid.New(), OrganizationID: prepOrganizations(t, db, 1)[0]},
				Err:              assert.AnError,
			}
		},
		"Duplicate name in the organization": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				Group: variable.Group{
					ID:             xid.New(),
					OrganizationID: g.OrganizationID,
					GroupName:      g.GroupName,
					Color:          g.Color,
					CreatedAt:      g.CreatedAt,
				},
				Err: variable.ErrDuplicateGroupName,
			}
		},
		"First group of the organization": func(t *testing.T, db *DB) tcase {
			return tcase{
				Group: variable.Group{
					ID:             xid.New(),
					OrganizationID: prepOrganizations(t, db, 1)[0],
					GroupName:      "Environment",
					Color:          "green",
					Builtin:        true,
					CreatedAt:      timeutil.Now().Truncate(time.Second),
					CreatedBy:      null.StringFrom(prepUsers(t, db, 1)[0]),
				},
			}
		},
		"Group after a gap": func(t *testing.T, db *DB) tcase {
			groups := prepVariableGroups(t, db, 2, func(i int, g *variable.Group) {
				g.SortIndex = i * 5
			})

			return tcase{
				Group: variable.Group{
					ID:             xid.New(),
					OrganizationID: groups[0].OrganizationID,
					GroupName:      "Environment",
					Color:          "green",
					CreatedAt:      timeutil.Now().Truncate(time.Second),
				},
				SortIndex: 6,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			err := db.CreateVariableGroup(ctx, c.Group)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			stored, err := db.FetchVariableGroup(context.Background(), c.Group.ID, c.Group.OrganizationID)
			require.NoError(t, err)

			exp := c.Group
			exp.SortIndex = c.SortIndex

			assert.Equal(t, exp, *stored)
		})
	}
}

func Test_agent_CreateVariable(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Variable         variable.Variable
		Options          []variable.Option
		SortIndex        int
		StoredOptions    []variable.Option
		Check            func(t *testing.T, db *DB)
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Variable:         variable.Variable{ID: xid.New(), OrganizationID: g.OrganizationID, GroupID: g.ID},
				Err:              assert.AnError,
			}
		},
		"Duplicate key in the organization": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			dup := v
			dup.ID = xid.New()

			return tcase{
				Variable: dup,
				Err:      variable.ErrDuplicateKey,
			}
		},
		"Option of an unknown variable": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				Variable: variable.Variable{
					ID:             xid.New(),
					OrganizationID: g.OrganizationID,
					GroupID:        g.ID,
					Key:            "region",
					Type:           variable.TypeList,
				},
				Options: []variable.Option{{ID: xid.New(), VariableID: xid.New(), Value: "a"}},
				Err:     assert.AnError,
			}
		},
		"Option id held by another variable": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]
			o := prepVariableOptions(t, db, v.ID, "a")[0]

			nv := v
			nv.ID = xid.New()
			nv.Key = "fresh"
			nv.SortIndex = 0

			// the upsert leaves the other variable's option alone, so the
			// short write is refused and the new variable rolled back.
			return tcase{
				Variable: nv,
				Options:  []variable.Option{{ID: o.ID, VariableID: nv.ID, Value: "b"}},
				Check: func(t *testing.T, db *DB) {
					t.Helper()

					assert.Equal(t, []variable.Option{o}, storedVariableOptions(t, db, v.ID))

					_, err := db.FetchVariable(context.Background(), nv.ID, nv.OrganizationID)
					assert.Equal(t, sql.ErrNoRows, err)
				},
				Err: variable.ErrUnknownOption,
			}
		},
		"List with options after siblings": func(t *testing.T, db *DB) tcase {
			siblings := prepVariables(t, db, 2, func(i int, v *variable.Variable) {
				v.SortIndex = i * 3
			})

			id := xid.New()

			options := []variable.Option{
				{ID: xid.New(), VariableID: id, Value: "eu-west-1"},
				{ID: xid.New(), VariableID: id, Value: "us-east-1", SortIndex: 1},
			}

			return tcase{
				Variable: variable.Variable{
					ID:             id,
					OrganizationID: siblings[0].OrganizationID,
					GroupID:        siblings[0].GroupID,
					Key:            "region",
					Name:           "Region",
					Description:    null.StringFrom("Cloud region"),
					Type:           variable.TypeList,
					DefaultValue:   null.StringFrom(options[0].ID.String()),
					Builtin:        true,
					CreatedAt:      timeutil.Now().Truncate(time.Second),
					CreatedBy:      null.StringFrom(prepUsers(t, db, 1)[0]),
				},
				Options:       options,
				SortIndex:     4,
				StoredOptions: options,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			v := c.Variable
			v.Options = c.Options

			err := db.CreateVariable(ctx, v)
			testutil.AssertEqualError(t, c.Err, err)

			if c.Check != nil {
				c.Check(t, db)
			}

			if err != nil {
				return
			}

			stored, err := db.FetchVariable(context.Background(), c.Variable.ID, c.Variable.OrganizationID)
			require.NoError(t, err)

			exp := c.Variable
			exp.SortIndex = c.SortIndex
			exp.Options = c.StoredOptions

			assert.Equal(t, exp, *stored)
		})
	}
}

func Test_agent_FetchVariableGroup(t *testing.T) {
	type tcase struct {
		ID             xid.ID
		OrganizationID string
		Result         *variable.Group
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Group of another organization": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				ID:             g.ID,
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Err:            sql.ErrNoRows,
			}
		},
		"Successful fetch": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				ID:             g.ID,
				OrganizationID: g.OrganizationID,
				Result:         &g,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			res, err := db.FetchVariableGroup(context.Background(), c.ID, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_FetchVariableGroups(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		Result           []variable.Group
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				OrganizationID:   prepOrganizations(t, db, 1)[0],
				Err:              assert.AnError,
			}
		},
		"No groups": func(t *testing.T, db *DB) tcase {
			prepVariableGroups(t, db, 1, nil)

			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Result:         []variable.Group{},
			}
		},
		"Built-in first then by sort index": func(t *testing.T, db *DB) tcase {
			groups := prepVariableGroups(t, db, 3, func(i int, g *variable.Group) {
				g.SortIndex = 2 - i
				g.Builtin = i == 0
			})

			return tcase{
				OrganizationID: groups[0].OrganizationID,
				Result:         []variable.Group{groups[0], groups[2], groups[1]},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			res, err := db.FetchVariableGroups(ctx, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_FetchVariable(t *testing.T) {
	type tcase struct {
		ID             xid.ID
		OrganizationID string
		Result         *variable.Variable
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Variable of another organization": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			return tcase{
				ID:             v.ID,
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Err:            sql.ErrNoRows,
			}
		},
		"Successful fetch with options": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]
			v.Options = prepVariableOptions(t, db, v.ID, "a", "b")

			return tcase{
				ID:             v.ID,
				OrganizationID: v.OrganizationID,
				Result:         &v,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			res, err := db.FetchVariable(context.Background(), c.ID, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_FetchVariablesByGroupID(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		GroupID          xid.ID
		OrganizationID   string
		Result           []variable.Variable
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				GroupID:          g.ID,
				OrganizationID:   g.OrganizationID,
				Err:              assert.AnError,
			}
		},
		"Empty group": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				GroupID:        g.ID,
				OrganizationID: g.OrganizationID,
				Result:         []variable.Variable{},
			}
		},
		"Variables in order": func(t *testing.T, db *DB) tcase {
			vars := prepVariables(t, db, 2, func(i int, v *variable.Variable) {
				v.SortIndex = 1 - i
			})

			vars[0].Options = prepVariableOptions(t, db, vars[0].ID, "a")

			// a variable in another group of the same organization stays
			// out of the listing.
			prepVariables(t, db, 1, func(_ int, v *variable.Variable) {
				v.OrganizationID = vars[0].OrganizationID
			})

			return tcase{
				GroupID:        vars[0].GroupID,
				OrganizationID: vars[0].OrganizationID,
				Result:         []variable.Variable{vars[1], vars[0]},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			res, err := db.FetchVariablesByGroupID(ctx, c.GroupID, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_fetchVariableOptions(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		VariableIDs      []xid.ID
		OrganizationID   string
		Result           map[xid.ID][]variable.Option
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				VariableIDs:      []xid.ID{v.ID},
				OrganizationID:   v.OrganizationID,
				Err:              assert.AnError,
			}
		},
		"Variable of another organization": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]
			prepVariableOptions(t, db, v.ID, "a")

			return tcase{
				VariableIDs:    []xid.ID{v.ID},
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Result:         map[xid.ID][]variable.Option{v.ID: {}},
			}
		},
		"Options in order by variable": func(t *testing.T, db *DB) tcase {
			vars := prepVariables(t, db, 2, nil)
			options := prepVariableOptions(t, db, vars[0].ID, "a", "b")

			return tcase{
				VariableIDs:    []xid.ID{vars[0].ID, vars[1].ID},
				OrganizationID: vars[0].OrganizationID,
				Result:         map[xid.ID][]variable.Option{vars[0].ID: options, vars[1].ID: {}},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			res, err := db.fetchVariableOptions(ctx, c.OrganizationID, c.VariableIDs)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_FetchVariableTree(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Fail             string
		OrganizationID   string
		UserID           string
		Result           variable.Tree
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				OrganizationID:   prepOrganizations(t, db, 1)[0],
				Err:              assert.AnError,
			}
		},
		"Missing values table": func(t *testing.T, db *DB) tcase {
			return tcase{
				Fail:           "personal_variable_values",
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Err:            assert.AnError,
			}
		},
		"Missing options table": func(t *testing.T, db *DB) tcase {
			return tcase{
				Fail:           "personal_variable_options",
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Err:            assert.AnError,
			}
		},
		"Empty organization": func(t *testing.T, db *DB) tcase {
			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Result:         variable.Tree{},
			}
		},
		"Groups with variables and options and the values of the user": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 2)

			groups := prepVariableGroups(t, db, 2, nil)

			vars := prepVariables(t, db, 2, func(i int, v *variable.Variable) {
				v.OrganizationID = groups[0].OrganizationID
				v.GroupID = groups[0].ID
				v.SortIndex = 1 - i
			})

			options := prepVariableOptions(t, db, vars[0].ID, "a", "b")
			vars[0].Options = options

			prepVariableValue(t, db, vars[0], users[0], options[1].ID.String())
			prepVariableValue(t, db, vars[1], users[1], "someone else's")

			return tcase{
				OrganizationID: groups[0].OrganizationID,
				UserID:         users[0],
				Result: variable.Tree{
					{
						Group: groups[0],
						Variables: []variable.Entry{
							{Variable: vars[1]},
							{Variable: vars[0], Value: null.StringFrom(options[1].ID.String())},
						},
					},
					{Group: groups[1], Variables: []variable.Entry{}},
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			if c.Fail != "" {
				_, err := db.sql.Exec("DROP TABLE " + c.Fail)
				require.NoError(t, err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			res, err := db.FetchVariableTree(ctx, c.OrganizationID, c.UserID)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_UpdateVariableGroup(t *testing.T) {
	type tcase struct {
		OrganizationID string
		Group          variable.Group
		Input          variable.GroupUpdateInput
		Name           string
		Color          string
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Nothing to set": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: g.OrganizationID,
				Group:          g,
				Name:           g.GroupName,
				Color:          g.Color,
				Err:            variable.ErrEmptyGroupUpdate,
			}
		},
		"Group of another organization": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Group:          g,
				Input:          variable.GroupUpdateInput{GroupName: null.StringFrom("Deploys")},
				Name:           g.GroupName,
				Color:          g.Color,
				Err:            errutil.ErrNotFound,
			}
		},
		"Duplicate name in the organization": func(t *testing.T, db *DB) tcase {
			groups := prepVariableGroups(t, db, 2, nil)

			return tcase{
				OrganizationID: groups[0].OrganizationID,
				Group:          groups[0],
				Input:          variable.GroupUpdateInput{GroupName: null.StringFrom(groups[1].GroupName)},
				Name:           groups[0].GroupName,
				Color:          groups[0].Color,
				Err:            variable.ErrDuplicateGroupName,
			}
		},
		"Renames and recolours": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: g.OrganizationID,
				Group:          g,
				Input: variable.GroupUpdateInput{
					GroupName: null.StringFrom("Deploys"),
					Color:     null.StringFrom("blue"),
				},
				Name:  "Deploys",
				Color: "blue",
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpdateVariableGroup(context.Background(), c.OrganizationID, c.Group.ID, c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			stored, err := db.FetchVariableGroup(context.Background(), c.Group.ID, c.Group.OrganizationID)
			require.NoError(t, err)
			assert.Equal(t, c.Name, stored.GroupName)
			assert.Equal(t, c.Color, stored.Color)
		})
	}
}

func Test_agent_UpdateVariableGroupOrder(t *testing.T) {
	type tcase struct {
		OrganizationID string
		IDs            []xid.ID
		Groups         []variable.Group
		Result         []xid.ID
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Group of another organization": func(t *testing.T, db *DB) tcase {
			groups := prepVariableGroups(t, db, 2, nil)
			other := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: groups[0].OrganizationID,
				IDs:            []xid.ID{groups[1].ID, other.ID, groups[0].ID},
				Groups:         groups,
				Result:         []xid.ID{groups[0].ID, groups[1].ID},
				Err:            errutil.ErrNotFound,
			}
		},
		"Successful reorder": func(t *testing.T, db *DB) tcase {
			groups := prepVariableGroups(t, db, 3, nil)

			return tcase{
				OrganizationID: groups[0].OrganizationID,
				IDs:            []xid.ID{groups[2].ID, groups[0].ID, groups[1].ID},
				Groups:         groups,
				Result:         []xid.ID{groups[2].ID, groups[0].ID, groups[1].ID},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpdateVariableGroupOrder(context.Background(), c.OrganizationID, c.IDs)
			testutil.AssertEqualError(t, c.Err, err)

			groups, err := db.FetchVariableGroups(context.Background(), c.OrganizationID)
			require.NoError(t, err)

			ids := make([]xid.ID, 0, len(groups))

			for _, g := range groups {
				ids = append(ids, g.ID)
			}

			assert.Equal(t, c.Result, ids)
		})
	}
}

func Test_agent_UpdateVariable(t *testing.T) {
	type tcase struct {
		Variable  variable.Variable
		Result    variable.Variable
		SortIndex int
		Err       error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Variable of another organization": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			moved := v
			moved.OrganizationID = prepOrganizations(t, db, 1)[0]
			moved.Name = "Renamed"

			return tcase{
				Variable: moved,
				Result:   v,
				Err:      errutil.ErrNotFound,
			}
		},
		"Duplicate key in the organization": func(t *testing.T, db *DB) tcase {
			vars := prepVariables(t, db, 2, nil)

			dup := vars[0]
			dup.Key = vars[1].Key

			return tcase{
				Variable: dup,
				Result:   vars[0],
				Err:      variable.ErrDuplicateKey,
			}
		},
		"Same group keeps its position": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 2, nil)[1]

			nv := v
			nv.Key = "renamed"
			nv.Name = "Renamed"
			nv.Description = null.StringFrom("Now described")
			nv.Type = variable.TypeNumber
			nv.DefaultValue = null.StringFrom("2")

			return tcase{
				Variable: nv,
				Result:   nv,
			}
		},
		"Another group puts it at the end": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			target := prepVariables(t, db, 2, func(i int, tv *variable.Variable) {
				tv.OrganizationID = v.OrganizationID
				tv.SortIndex = i * 4
			})

			nv := v
			nv.GroupID = target[0].GroupID

			res := nv
			res.SortIndex = 5

			return tcase{
				Variable: nv,
				Result:   res,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpdateVariable(context.Background(), c.Variable)
			testutil.AssertEqualError(t, c.Err, err)

			stored, err := db.FetchVariable(context.Background(), c.Variable.ID, c.Result.OrganizationID)
			require.NoError(t, err)
			assert.Equal(t, c.Result, *stored)
		})
	}
}

func Test_agent_UpdateVariableDefault(t *testing.T) {
	type tcase struct {
		Fail     string
		Variable variable.Variable
		Result   variable.Variable
		Err      error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Variable of another organization": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			moved := v
			moved.OrganizationID = prepOrganizations(t, db, 1)[0]
			moved.DefaultValue = null.StringFrom("changed")

			return tcase{
				Variable: moved,
				Result:   v,
				Err:      errutil.ErrNotFound,
			}
		},
		"Type changed since the read": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			stale := v
			stale.Type = variable.TypeNumber
			stale.DefaultValue = null.StringFrom("2")

			return tcase{
				Variable: stale,
				Result:   v,
				Err:      errutil.ErrNotFound,
			}
		},
		"Chosen option removed meanwhile": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, func(_ int, v *variable.Variable) {
				v.Type = variable.TypeList
			})[0]
			v.Options = prepVariableOptions(t, db, v.ID, "a")

			stale := v
			stale.DefaultValue = null.StringFrom(xid.New().String())

			return tcase{
				Variable: stale,
				Result:   v,
				Err:      errutil.ErrNotFound,
			}
		},
		"Missing options table": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, func(_ int, v *variable.Variable) {
				v.Type = variable.TypeList
			})[0]

			nv := v
			nv.Options = []variable.Option{{ID: xid.New(), VariableID: v.ID, Value: "a"}}

			return tcase{
				Fail:     "personal_variable_options",
				Variable: nv,
				Err:      assert.AnError,
			}
		},
		"List default and option order only": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, func(_ int, v *variable.Variable) {
				v.Type = variable.TypeList
			})[0]
			options := prepVariableOptions(t, db, v.ID, "a", "b")

			nv := v
			nv.Name = "Ignored"
			nv.DefaultValue = null.StringFrom(options[1].ID.String())
			nv.Options = []variable.Option{
				{ID: options[1].ID, VariableID: v.ID, Value: "b"},
				{ID: options[0].ID, VariableID: v.ID, Value: "a", SortIndex: 1},
				{ID: xid.New(), VariableID: v.ID, Value: "removed meanwhile", SortIndex: 2},
			}

			res := v
			res.DefaultValue = nv.DefaultValue
			res.Options = nv.Options[:2]

			return tcase{
				Variable: nv,
				Result:   res,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			if c.Fail != "" {
				_, err := db.sql.Exec("DROP TABLE " + c.Fail)
				require.NoError(t, err)
			}

			err := db.UpdateVariableDefault(context.Background(), c.Variable)
			testutil.AssertEqualError(t, c.Err, err)

			if c.Fail != "" {
				return
			}

			stored, err := db.FetchVariable(context.Background(), c.Result.ID, c.Result.OrganizationID)
			require.NoError(t, err)
			assert.Equal(t, c.Result, *stored)
		})
	}
}

func Test_agent_UpdateVariableOrder(t *testing.T) {
	type tcase struct {
		OrganizationID string
		GroupID        xid.ID
		IDs            []xid.ID
		Result         []xid.ID
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Variable of another organization": func(t *testing.T, db *DB) tcase {
			vars := prepVariables(t, db, 2, nil)
			other := prepVariables(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: vars[0].OrganizationID,
				GroupID:        vars[0].GroupID,
				IDs:            []xid.ID{vars[1].ID, other.ID, vars[0].ID},
				Result:         []xid.ID{vars[0].ID, vars[1].ID},
				Err:            errutil.ErrNotFound,
			}
		},
		"Variable moved in from another group": func(t *testing.T, db *DB) tcase {
			vars := prepVariables(t, db, 2, nil)

			moved := prepVariables(t, db, 1, func(_ int, v *variable.Variable) {
				v.OrganizationID = vars[0].OrganizationID
			})[0]

			return tcase{
				OrganizationID: vars[0].OrganizationID,
				GroupID:        vars[0].GroupID,
				IDs:            []xid.ID{vars[1].ID, moved.ID, vars[0].ID},
				Result:         []xid.ID{vars[1].ID, moved.ID, vars[0].ID},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpdateVariableOrder(context.Background(), c.OrganizationID, c.GroupID, c.IDs)
			testutil.AssertEqualError(t, c.Err, err)

			vars, err := db.FetchVariablesByGroupID(context.Background(), c.GroupID, c.OrganizationID)
			require.NoError(t, err)

			ids := make([]xid.ID, 0, len(vars))

			for _, v := range vars {
				ids = append(ids, v.ID)
			}

			assert.Equal(t, c.Result, ids)
		})
	}
}

func Test_agent_ReplaceVariableOptions(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Fail             string
		OrganizationID   string
		Variable         variable.Variable
		Options          []variable.Option
		Result           []variable.Option
		Values           map[string]string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				OrganizationID:   v.OrganizationID,
				Variable:         v,
				Result:           []variable.Option{},
				Values:           map[string]string{},
				Err:              assert.AnError,
			}
		},
		"Missing values table": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]
			options := prepVariableOptions(t, db, v.ID, "a")

			return tcase{
				Fail:           "personal_variable_values",
				OrganizationID: v.OrganizationID,
				Variable:       v,
				Result:         options,
				Err:            assert.AnError,
			}
		},
		"Variable of another organization": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]
			options := prepVariableOptions(t, db, v.ID, "a")

			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Variable:       v,
				Result:         options,
				Values:         map[string]string{},
				Err:            sql.ErrNoRows,
			}
		},
		"Kept renamed reordered and removed options": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 2)

			v := prepVariables(t, db, 1, func(_ int, v *variable.Variable) {
				v.Type = variable.TypeList
			})[0]

			options := prepVariableOptions(t, db, v.ID, "a", "b", "c")

			prepVariableValue(t, db, v, users[0], options[0].ID.String())
			prepVariableValue(t, db, v, users[1], options[1].ID.String())

			replaced := []variable.Option{
				{ID: options[2].ID, VariableID: v.ID, Value: "c2"},
				{ID: xid.New(), VariableID: v.ID, Value: "d", SortIndex: 1},
				{ID: options[0].ID, VariableID: v.ID, Value: "a", SortIndex: 2},
			}

			return tcase{
				OrganizationID: v.OrganizationID,
				Variable:       v,
				Options:        replaced,
				Result:         replaced,
				Values:         map[string]string{users[0]: options[0].ID.String()},
			}
		},
		"No options left": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)

			v := prepVariables(t, db, 1, nil)[0]
			options := prepVariableOptions(t, db, v.ID, "a")

			prepVariableValue(t, db, v, users[0], options[0].ID.String())

			return tcase{
				OrganizationID: v.OrganizationID,
				Variable:       v,
				Result:         []variable.Option{},
				Values:         map[string]string{},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			if c.Fail != "" {
				_, err := db.sql.Exec("DROP TABLE " + c.Fail)
				require.NoError(t, err)
			}

			v := c.Variable
			v.OrganizationID = c.OrganizationID
			v.Options = c.Options

			err := db.ReplaceVariableOptions(ctx, v)
			testutil.AssertEqualError(t, c.Err, err)

			if c.Fail != "personal_variable_options" {
				assert.Equal(t, c.Result, storedVariableOptions(t, db, c.Variable.ID))
			}

			if c.Fail != "" {
				return
			}

			assert.Equal(t, c.Values, storedVariableValues(t, db, c.Variable.ID))
		})
	}
}

func Test_agent_UpsertVariableValue(t *testing.T) {
	type tcase struct {
		OrganizationID string
		UserID         string
		Variable       variable.Variable
		Value          string
		Result         map[string]string
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Variable of another organization": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				UserID:         prepUsers(t, db, 1)[0],
				Variable:       v,
				Value:          "a",
				Result:         map[string]string{},
				Err:            errutil.ErrNotFound,
			}
		},
		"First value": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 2)
			v := prepVariables(t, db, 1, nil)[0]

			prepVariableValue(t, db, v, users[1], "theirs")

			return tcase{
				OrganizationID: v.OrganizationID,
				UserID:         users[0],
				Variable:       v,
				Value:          "mine",
				Result:         map[string]string{users[0]: "mine", users[1]: "theirs"},
			}
		},
		"Replaced value": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)
			v := prepVariables(t, db, 1, nil)[0]

			prepVariableValue(t, db, v, users[0], "old")

			return tcase{
				OrganizationID: v.OrganizationID,
				UserID:         users[0],
				Variable:       v,
				Value:          "new",
				Result:         map[string]string{users[0]: "new"},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpsertVariableValue(context.Background(), c.OrganizationID, c.UserID, c.Variable.ID, c.Value)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, storedVariableValues(t, db, c.Variable.ID))
		})
	}
}

func Test_agent_DeleteVariableGroup(t *testing.T) {
	type tcase struct {
		OrganizationID string
		Group          variable.Group
		Exists         bool
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Group of another organization": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Group:          g,
				Exists:         true,
				Err:            errutil.ErrNotFound,
			}
		},
		"Group with variables": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: v.OrganizationID,
				Group:          variable.Group{ID: v.GroupID, OrganizationID: v.OrganizationID},
				Exists:         true,
				Err:            variable.ErrGroupNotEmpty,
			}
		},
		"Empty group": func(t *testing.T, db *DB) tcase {
			g := prepVariableGroups(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: g.OrganizationID,
				Group:          g,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.DeleteVariableGroup(context.Background(), c.Group.ID, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)

			_, err = db.FetchVariableGroup(context.Background(), c.Group.ID, c.Group.OrganizationID)
			assert.Equal(t, c.Exists, err == nil)
		})
	}
}

func Test_agent_DeleteVariable(t *testing.T) {
	type tcase struct {
		OrganizationID string
		Variable       variable.Variable
		Exists         bool
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Variable of another organization": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]

			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Variable:       v,
				Exists:         true,
				Err:            errutil.ErrNotFound,
			}
		},
		"Variable with options and values": func(t *testing.T, db *DB) tcase {
			v := prepVariables(t, db, 1, nil)[0]
			options := prepVariableOptions(t, db, v.ID, "a")

			prepVariableValue(t, db, v, prepUsers(t, db, 1)[0], options[0].ID.String())

			return tcase{
				OrganizationID: v.OrganizationID,
				Variable:       v,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.DeleteVariable(context.Background(), c.Variable.ID, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)

			_, err = db.FetchVariable(context.Background(), c.Variable.ID, c.Variable.OrganizationID)
			assert.Equal(t, c.Exists, err == nil)

			if c.Exists {
				return
			}

			assert.Empty(t, storedVariableOptions(t, db, c.Variable.ID))
			assert.Empty(t, storedVariableValues(t, db, c.Variable.ID))
		})
	}
}

func Test_agent_DeleteVariableValue(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		UserID           string
		Variable         variable.Variable
		Result           map[string]string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)
			v := prepVariables(t, db, 1, nil)[0]

			prepVariableValue(t, db, v, users[0], "mine")

			return tcase{
				CancelledContext: true,
				OrganizationID:   v.OrganizationID,
				UserID:           users[0],
				Variable:         v,
				Result:           map[string]string{users[0]: "mine"},
				Err:              assert.AnError,
			}
		},
		"Only the value of the user": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 2)
			v := prepVariables(t, db, 1, nil)[0]

			prepVariableValue(t, db, v, users[0], "mine")
			prepVariableValue(t, db, v, users[1], "theirs")

			return tcase{
				OrganizationID: v.OrganizationID,
				UserID:         users[0],
				Variable:       v,
				Result:         map[string]string{users[1]: "theirs"},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			err := db.DeleteVariableValue(ctx, c.OrganizationID, c.UserID, c.Variable.ID)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, storedVariableValues(t, db, c.Variable.ID))
		})
	}
}

func Test_agent_DeleteVariableValues(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		Variable         variable.Variable
		Result           map[string]string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)
			v := prepVariables(t, db, 1, nil)[0]

			prepVariableValue(t, db, v, users[0], "mine")

			return tcase{
				CancelledContext: true,
				OrganizationID:   v.OrganizationID,
				Variable:         v,
				Result:           map[string]string{users[0]: "mine"},
				Err:              assert.AnError,
			}
		},
		"Values of every user": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 2)
			v := prepVariables(t, db, 1, nil)[0]

			prepVariableValue(t, db, v, users[0], "mine")
			prepVariableValue(t, db, v, users[1], "theirs")

			return tcase{
				OrganizationID: v.OrganizationID,
				Variable:       v,
				Result:         map[string]string{},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			err := db.DeleteVariableValues(ctx, c.OrganizationID, c.Variable.ID)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, storedVariableValues(t, db, c.Variable.ID))
		})
	}
}
