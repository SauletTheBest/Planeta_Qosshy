package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// UniqueViolation reports whether err is Postgres error 23505 ("duplicate key value
// violates unique constraint") and returns the name of the constraint that refused
// the row, e.g. "users_email_key".
//
// Checking "does it already exist?" with a SELECT before an INSERT is not enough:
// two requests at the same moment both see "no" and both insert. The UNIQUE index
// is the real guard; this helper only turns its error into a friendly message.
func UniqueViolation(err error) (constraint string, ok bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName, true
	}
	return "", false
}
