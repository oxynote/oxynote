package sqlutil

import (
	"fmt"

	sq "github.com/Masterminds/squirrel"
)

// SelectIsIn is a helper function that allows to perform an in check query as
// a sub query for where statements.
func SelectIsIn(column string, query sq.SelectBuilder) sq.Sqlizer { //nolint:ireturn // squirrel returns an interface.
	sqlStr, args, _ := query.ToSql()
	return sq.Expr(fmt.Sprintf("%s IN (%s)", column, sqlStr), args...)
}

// SelectIsNotIn is a helper function that allows to perform a not in check query as
// a sub query for where statements.
func SelectIsNotIn(column string, query sq.SelectBuilder) sq.Sqlizer { //nolint:ireturn // squirrel returns an interface.
	sqlStr, args, _ := query.ToSql()
	return sq.Expr(fmt.Sprintf("%s NOT IN (%s)", column, sqlStr), args...)
}

// SelectIsEqual is a helper function that allows to perform an equals query as
// a sub query for where statements.
func SelectIsEqual(column string, query sq.SelectBuilder) sq.Sqlizer { //nolint:ireturn // squirrel returns an interface.
	sqlStr, args, _ := query.ToSql()
	return sq.Expr(fmt.Sprintf("%s = (%s)", column, sqlStr), args...)
}

// SelectIsNotEqual is a helper function that allows to perform an equals query
// as a sub query for where statements.
func SelectIsNotEqual(column string, query sq.SelectBuilder) sq.Sqlizer { //nolint:ireturn // squirrel returns an interface.
	sqlStr, args, _ := query.ToSql()
	return sq.Expr(fmt.Sprintf("%s != (%s)", column, sqlStr), args...)
}

// SelectIsNull is a helper function that allows to perform a null check query
// as a sub query for where statements.
func SelectIsNull(query sq.SelectBuilder) sq.Sqlizer { //nolint:ireturn // squirrel returns an interface.
	sqlStr, args, _ := query.ToSql()
	return sq.Expr(fmt.Sprintf("(%s) IS NULL", sqlStr), args...)
}

// SelectIsNotNull is a helper function that allows to perform a not null
// check query as a sub query for where statements.
func SelectIsNotNull(query sq.SelectBuilder) sq.Sqlizer { //nolint:ireturn // squirrel returns an interface.
	sqlStr, args, _ := query.ToSql()
	return sq.Expr(fmt.Sprintf("(%s) IS NOT NULL", sqlStr), args...)
}
