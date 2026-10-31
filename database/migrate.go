package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"planeta_qosshy/migrations"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// runMigrations applies every migrations/*.up.sql the database hasn't seen yet.
// Applied versions are stored in the schema_migrations table, and a Postgres
// advisory lock stops two app instances from migrating at the same time.
//
// It opens its own connection on purpose: migrate's Close() also closes the
// *sql.DB it was given, so sharing GORM's pool would leave the app without one.
func runMigrations(dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		sqlDB.Close()
		return fmt.Errorf("open embedded migrations: %w", err)
	}

	driver, err := pgxmigrate.WithInstance(sqlDB, &pgxmigrate.Config{})
	if err != nil {
		sqlDB.Close()
		return fmt.Errorf("migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		sqlDB.Close()
		return err
	}
	defer m.Close() // closes the source, the driver and sqlDB

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	version, dirty, err := m.Version()
	if err != nil {
		return err
	}
	log.Printf("Database schema is at version %d (dirty=%v)", version, dirty)
	return nil
}
