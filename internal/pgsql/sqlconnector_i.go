package pgsql

import (
	"context"
	"database/sql"
	"time"

	ilog "mcmm/internal/log"

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
	logger := ilog.Component("pgsql")
	logger.Infof("opening database connection")
	db, err := sql.Open("pgx", c.dsn)
	if err != nil {
		logger.Errorf("sql.Open failed: %v", err)
		return err
	}
	c.db = db
	logger.Infof("pinging database")
	if err := c.db.PingContext(ctx); err != nil {
		logger.Errorf("ping failed: %v", err)
		return err
	}
	logger.Infof("database connection ready")
	return nil
}

func (c *Connector) Close() error {
	logger := ilog.Component("pgsql")
	if c.db == nil {
		logger.Warnf("close skipped (db is nil)")
		return nil
	}
	logger.Infof("closing database connection")
	return c.db.Close()
}

func (c *Connector) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.db.QueryRowContext(ctx, query, args...)
}

func (c *Connector) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, query, args...)
}

func (c *Connector) PingContext(ctx context.Context) error {
	logger := ilog.Component("pgsql")
	if c.db == nil {
		logger.Warnf("ping requested but db is nil")
		return sql.ErrConnDone
	}
	logger.Debugf("pinging database")
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
