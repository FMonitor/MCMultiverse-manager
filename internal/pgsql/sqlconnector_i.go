package pgsql

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type SQLConnector interface {
	Connect(ctx context.Context) error
	Close() error
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PingContext(ctx context.Context) error
	SetMaxOpenConns(n int)
	SetMaxIdleConns(n int)
	SetConnMaxLifetime(d time.Duration)
}

type Connector struct {
	dsn string
	db  *sql.DB
}

func NewConnector(dsn string) *Connector {
	return &Connector{dsn: dsn}
}

func (c *Connector) Connect(ctx context.Context) error {
	db, err := sql.Open("pgx", c.dsn)
	if err != nil {
		return err
	}
	c.db = db
	return c.db.PingContext(ctx)
}

func (c *Connector) Close() error {
	if c.db == nil {
		return nil
	}
	return c.db.Close()
}

func (c *Connector) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.db.QueryRowContext(ctx, query, args...)
}

func (c *Connector) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, query, args...)
}

func (c *Connector) PingContext(ctx context.Context) error {
	if c.db == nil {
		return sql.ErrConnDone
	}
	return c.db.PingContext(ctx)
}

func (c *Connector) SetMaxOpenConns(n int) {
	if c.db != nil {
		c.db.SetMaxOpenConns(n)
	}
}

func (c *Connector) SetMaxIdleConns(n int) {
	if c.db != nil {
		c.db.SetMaxIdleConns(n)
	}
}

func (c *Connector) SetConnMaxLifetime(d time.Duration) {
	if c.db != nil {
		c.db.SetConnMaxLifetime(d)
	}
}
