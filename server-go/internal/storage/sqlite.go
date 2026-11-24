package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	sqlite "modernc.org/sqlite"
)

//go:embed schema/server.sql
var schemaSQL string

const (
	systemPlayerViewRange int64 = 15

	// Player job bits are warrior=1, taoist=2, wizard=4.
	validJobMask int64 = 7
)

var sqliteDriverSequence atomic.Uint64

// OpenOptions configures a legacy-compatible SQLite store.
type OpenOptions struct {
	Path              string
	CreateIfMissing   bool
	BusyTimeout       time.Duration
	ItemCategoryMatch func(itemID, category int64) bool
}

// Store owns the SQLite connection pool and legacy schema.
type Store struct {
	db         *sql.DB
	path       string
	dsn        string
	driverName string
}

// Open opens an existing mir2x database or creates the exact legacy schema.
func Open(ctx context.Context, options OpenOptions) (*Store, error) {
	if strings.TrimSpace(options.Path) == "" {
		return nil, errors.New("database path is empty")
	}
	path := options.Path
	isMemory := path == ":memory:"
	if !isMemory {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve database path: %w", err)
		}
		path = absolute
		if _, err := os.Stat(path); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("stat database: %w", err)
			}
			if !options.CreateIfMissing {
				return nil, fmt.Errorf("database does not exist: %s", path)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, fmt.Errorf("create database directory: %w", err)
			}
		}
	}

	if options.BusyTimeout <= 0 {
		options.BusyTimeout = 5 * time.Second
	}

	driverName := fmt.Sprintf("mir2x-sqlite-%d", sqliteDriverSequence.Add(1))
	drv := &sqlite.Driver{}
	if err := registerSQLiteFunctions(drv, options.ItemCategoryMatch); err != nil {
		return nil, err
	}
	sql.Register(driverName, drv)

	dsn, err := buildSQLiteDSN(path, options.BusyTimeout, isMemory)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	store := &Store{db: db, path: path, dsn: dsn, driverName: driverName}

	cleanupOnError := func(err error) (*Store, error) {
		_ = db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return cleanupOnError(fmt.Errorf("ping sqlite: %w", err))
	}
	if err := store.ensureSchema(ctx); err != nil {
		return cleanupOnError(err)
	}
	return store, nil
}

func registerSQLiteFunctions(drv *sqlite.Driver, itemCategoryMatch func(int64, int64) bool) error {
	if err := drv.RegisterDeterministicScalarFunction("jobValid", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if len(args) != 1 {
			return int64(0), nil
		}
		job, ok := args[0].(int64)
		if !ok || job <= 0 || job&^validJobMask != 0 {
			return int64(0), nil
		}
		return int64(1), nil
	}); err != nil {
		return fmt.Errorf("register jobValid: %w", err)
	}

	if err := drv.RegisterDeterministicScalarFunction("SYS_PLY_VIEWR", 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
		return systemPlayerViewRange, nil
	}); err != nil {
		return fmt.Errorf("register SYS_PLY_VIEWR: %w", err)
	}

	if err := drv.RegisterDeterministicScalarFunction("checkAuctionCategory", 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if len(args) != 2 {
			return int64(0), nil
		}
		itemID, okItem := args[0].(int64)
		category, okCategory := args[1].(int64)
		if !okItem || !okCategory || itemCategoryMatch == nil {
			return int64(0), nil
		}
		if itemCategoryMatch(itemID, category) {
			return int64(1), nil
		}
		return int64(0), nil
	}); err != nil {
		return fmt.Errorf("register checkAuctionCategory: %w", err)
	}
	return nil
}

func buildSQLiteDSN(path string, busyTimeout time.Duration, isMemory bool) (string, error) {
	var base string
	if isMemory {
		base = fmt.Sprintf("file:mir2x-memory-%d?mode=memory&cache=shared", sqliteDriverSequence.Load())
	} else {
		base = "file:" + filepath.ToSlash(path)
	}
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	milliseconds := busyTimeout.Milliseconds()
	if milliseconds <= 0 {
		milliseconds = 5000
	}
	return fmt.Sprintf("%s%s_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_txlock=immediate",
		base, separator, milliseconds), nil
}

func (s *Store) ensureSchema(ctx context.Context) error {
	var coreTables int
	if err := s.db.QueryRowContext(ctx, `select count(*) from sqlite_master where type='table' and name='tbl_account'`).Scan(&coreTables); err != nil {
		return fmt.Errorf("inspect schema: %w", err)
	}
	if coreTables == 0 {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin schema transaction: %w", err)
		}
		if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply schema: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit schema: %w", err)
		}
	}
	tables, err := s.TableNames(ctx)
	if err != nil {
		return err
	}
	found := make(map[string]bool, len(tables))
	for _, table := range tables {
		found[table] = true
	}
	var missing []string
	for _, table := range expectedTables {
		if !found[table] {
			missing = append(missing, table)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("database is missing tables: %s", strings.Join(missing, ", "))
	}
	return nil
}

// DB exposes the underlying database for repositories and transactions.
func (s *Store) DB() *sql.DB { return s.db }

// Path returns the resolved database path.
func (s *Store) Path() string { return s.path }

// DSN returns the SQLite DSN used by this store.
func (s *Store) DSN() string { return s.dsn }

// Close closes the database pool.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// TableNames returns user table names excluding SQLite internal tables.
func (s *Store) TableNames(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `select name from sqlite_master where type='table' and name not like 'sqlite_%' order by name`)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table: %w", err)
		}
		result = append(result, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tables: %w", err)
	}
	return result, nil
}

// InTx executes fn inside one transaction.
func (s *Store) InTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// SchemaSQL returns the embedded legacy schema for audit/testing.
func SchemaSQL() string { return schemaSQL }

var expectedTables = []string{
	"tbl_account",
	"tbl_auctionitem",
	"tbl_belt",
	"tbl_blacklist",
	"tbl_char",
	"tbl_charvarlist",
	"tbl_chatgroup",
	"tbl_chatgroupmember",
	"tbl_chatmessage",
	"tbl_delivery",
	"tbl_friend",
	"tbl_guild",
	"tbl_guildinvite",
	"tbl_guildmember",
	"tbl_guildwar",
	"tbl_inventory",
	"tbl_learnedmagiclist",
	"tbl_playerconfig",
	"tbl_secureditemlist",
	"tbl_stall",
	"tbl_stallitem",
	"tbl_wear",
}
