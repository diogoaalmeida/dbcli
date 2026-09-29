package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

type pgDriver struct{}

func init() {
	driver.Register("postgres", pgDriver{})
	driver.Register("postgresql", pgDriver{})
}

func (pgDriver) Connect(ctx context.Context, dsn string) (driver.Conn, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &conn{pool: pool}, nil
}
