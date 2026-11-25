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

// SetItemCategoryMatch 在开库之后补装「拍卖分类匹配器」。
//
// ★ 为什么要「补装」而不是开库时传进去：
//
//	分类匹配要查物品表（isWeapon / isDress / wearable(WLG_*)），
//	而物品表是 game 包从 content/items.json 载的，
//	**开库时还不知道物品表长什么样**（开库发生在载表之前）。
//
//	早期实现把 ItemCategoryMatch 放在 OpenOptions 里，但 main 从没传 →
//	checkAuctionCategory 恒返回 0 → 拍卖列表永远为空（见下面的长注释）。
//
//	现在用 atomic 存函数指针，开库后随时可换，且读写无锁竞争。
func SetItemCategoryMatch(fn func(itemID, category int64) bool) {
	if fn == nil {
		return
	}
	itemCategoryMatchFunc.Store(&fn)
}

// getItemCategoryMatch 读取当前分类匹配器（可能为 nil）。
func getItemCategoryMatch() func(itemID, category int64) bool {
	if p := itemCategoryMatchFunc.Load(); p != nil {
		return *p
	}
	return nil
}

var itemCategoryMatchFunc atomic.Pointer[func(itemID, category int64) bool]

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
		if !okItem || !okCategory {
			return int64(0), nil
		}
		// ★★ 这一行曾经把「拍卖列表永远为空」的 bug 藏了很久 ★★
		//
		// 早期实现：`if !okItem || !okCategory || itemCategoryMatch == nil { return 0 }`
		//
		// 而 main.go 从来没传 OpenOptions.ItemCategoryMatch ——
		// 于是这个函数**恒返回 0**，SQL 条件 `checkAuctionCategory(...) = 1`
		// 永远不成立，拍卖行**任何物品都查不出来**。
		//
		// 表现极具迷惑性：上架成功（写库 OK）、自买自检生效（Take 不带该条件）、
		// 只有「浏览列表」是空的 —— 玩家会以为「拍卖行没人挂东西」。
		//
		// 修法：category == ALL 时直接放行（原版 server.cpp:352
		// `if(category == AUCTIONCAT_ALL){ result_int(1); return; }` 在最前面）。
		// 只有需要按具体类别过滤时才依赖注入的匹配器。
		const auctionCatAll = 1
		if category == auctionCatAll {
			return int64(1), nil
		}
		// 优先用运行时补装的匹配器（见 SetItemCategoryMatch），
		// 其次退回开库时传的
		match := getItemCategoryMatch()
		if match == nil {
			match = itemCategoryMatch
		}
		if match == nil {
			// 没有分类器时不能假装「不匹配」—— 那会让整个拍卖列表空掉。
			// 退化成「全部类别都匹配」，至少玩家能看到东西。
			return int64(1), nil
		}
		if match(itemID, category) {
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
