package repository

import (
	"context"
	"database/sql"
	"errors"
	"payment-service/internal/domain"
)

type PostgresPaymentRepository struct {
	db *sql.DB
}

func NewPostgresPaymentRepository(db *sql.DB) *PostgresPaymentRepository {
	return &PostgresPaymentRepository{db: db}
}

func (r *PostgresPaymentRepository) EnsureSchema(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS payments (
			id             TEXT PRIMARY KEY,
			order_id       TEXT UNIQUE NOT NULL,
			transaction_id TEXT NOT NULL,
			amount         BIGINT NOT NULL,
			status         TEXT NOT NULL,
			customer_email TEXT NOT NULL DEFAULT ''
		)`,
		`ALTER TABLE payments ADD COLUMN IF NOT EXISTS customer_email TEXT NOT NULL DEFAULT ''`,
	}
	for _, q := range queries {
		if _, err := r.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresPaymentRepository) Create(ctx context.Context, payment *domain.Payment) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO payments (id, order_id, transaction_id, amount, status, customer_email)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		payment.ID, payment.OrderID, payment.TransactionID, payment.Amount, payment.Status, payment.CustomerEmail,
	)
	return err
}

func (r *PostgresPaymentRepository) GetByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, order_id, transaction_id, amount, status, customer_email
		 FROM payments WHERE order_id = $1`, orderID,
	)
	var p domain.Payment
	if err := row.Scan(&p.ID, &p.OrderID, &p.TransactionID, &p.Amount, &p.Status, &p.CustomerEmail); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrPaymentNotFound
		}
		return nil, err
	}
	return &p, nil
}
