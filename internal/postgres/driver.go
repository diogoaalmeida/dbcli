package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

type pgDriver struct{}

func init() {
	driver.Register("postgres", pgDriver{})
	driver.Register("postgresql", pgDriver{})
}

// Connect uses a single pgx.Conn rather than a pgxpool.Pool: dbcli connects
// once, runs one operation, and exits, so a connection pool (min/max size,
// idle eviction, multiple physical connections) is more machinery than a
// single-shot CLI needs.
func (pgDriver) Connect(ctx context.Context, dsn string) (driver.Conn, error) {
	pgxConn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &conn{pgxConn: pgxConn}, nil
}
