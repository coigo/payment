package infra

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/coigo/payment-processor-course/migrations"
	"github.com/pressly/goose/v3"
)

func Migrate (ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("migrate: failed to open database: %w", err)
	}

	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrate: failed to set dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("migrate: failed to migrate: %w", err)
	}
	return nil

	
} 