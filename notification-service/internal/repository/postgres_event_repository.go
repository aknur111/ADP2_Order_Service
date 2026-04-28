package repository

import (
	"context"
	"database/sql"
)

type PostgresEventRepository struct {
	db *sql.DB
}

func NewPostgresEventRepository(db *sql.DB) *PostgresEventRepository {
	return &PostgresEventRepository{db: db}
}

func (r *PostgresEventRepository) EnsureSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS processed_events (
			event_id     TEXT PRIMARY KEY,
			processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	return err
}

func (r *PostgresEventRepository) MarkProcessed(ctx context.Context, eventID string) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT (event_id) DO NOTHING`,
		eventID,
	)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}
