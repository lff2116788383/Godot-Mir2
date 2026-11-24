package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenCreatesExactSchema(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "mir2x.db3")
	store, err := Open(context.Background(), OpenOptions{Path: dbPath, CreateIfMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	tables, err := store.TableNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 22 {
		t.Fatalf("table count = %d, want 22: %v", len(tables), tables)
	}
	found := make(map[string]bool)
	for _, table := range tables {
		found[table] = true
	}
	for _, table := range expectedTables {
		if !found[table] {
			t.Fatalf("missing table %s", table)
		}
	}

	var indexes, triggers int
	if err := store.DB().QueryRow(`select count(*) from sqlite_master where type='index' and name not like 'sqlite_%'`).Scan(&indexes); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`select count(*) from sqlite_master where type='trigger'`).Scan(&triggers); err != nil {
		t.Fatal(err)
	}
	if indexes != 11 || triggers != 1 {
		t.Fatalf("indexes=%d triggers=%d, want 11/1", indexes, triggers)
	}
}

func TestSQLiteLegacyFunctionsAndDefaults(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), OpenOptions{
		Path:            filepath.Join(t.TempDir(), "functions.db3"),
		CreateIfMissing: true,
		ItemCategoryMatch: func(itemID, category int64) bool {
			return itemID == 100 && category == 2
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	var valid, invalid, nullValue int
	if err := store.DB().QueryRowContext(ctx, `select jobValid(1), jobValid(8), jobValid(null)`).Scan(&valid, &invalid, &nullValue); err != nil {
		t.Fatal(err)
	}
	if valid != 1 || invalid != 0 || nullValue != 0 {
		t.Fatalf("jobValid = %d/%d/%d", valid, invalid, nullValue)
	}

	var viewRange int
	if err := store.DB().QueryRowContext(ctx, `select SYS_PLY_VIEWR()`).Scan(&viewRange); err != nil {
		t.Fatal(err)
	}
	if viewRange != int(systemPlayerViewRange) {
		t.Fatalf("view range = %d", viewRange)
	}

	var match, noMatch int
	if err := store.DB().QueryRowContext(ctx, `select checkAuctionCategory(100,2), checkAuctionCategory(100,3)`).Scan(&match, &noMatch); err != nil {
		t.Fatal(err)
	}
	if match != 1 || noMatch != 0 {
		t.Fatalf("auction matcher = %d/%d", match, noMatch)
	}

	if _, err := store.DB().ExecContext(ctx, `insert into tbl_account(fld_account,fld_password) values('alice','pw')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `insert into tbl_char(fld_account,fld_name,fld_gender,fld_job,fld_map,fld_mapx,fld_mapy) values('alice','hero',1,1,0,10,20)`); err != nil {
		t.Fatal(err)
	}
	var dbid uint32
	var defaultView int
	if err := store.DB().QueryRowContext(ctx, `select fld_dbid, fld_viewr from tbl_char where fld_name='hero'`).Scan(&dbid, &defaultView); err != nil {
		t.Fatal(err)
	}
	if dbid == 0 || defaultView != int(systemPlayerViewRange) {
		t.Fatalf("character dbid=%d viewr=%d", dbid, defaultView)
	}

	if _, err := store.DB().ExecContext(ctx, `insert into tbl_char(fld_account,fld_name,fld_gender,fld_job,fld_map,fld_mapx,fld_mapy) values('missing','bad',1,1,0,0,0)`); err == nil {
		t.Fatal("expected foreign-key failure")
	}
	if _, err := store.DB().ExecContext(ctx, `insert into tbl_char(fld_account,fld_name,fld_gender,fld_job,fld_map,fld_mapx,fld_mapy) values('alice','badjob',1,8,0,0,0)`); err == nil {
		t.Fatal("expected jobValid CHECK failure")
	}
}

func TestReopenExistingDatabase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "reopen.db3")
	store, err := Open(ctx, OpenOptions{Path: dbPath, CreateIfMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `insert into tbl_account(fld_account,fld_password) values('alice','pw')`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, OpenOptions{Path: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var count int
	if err := reopened.DB().QueryRowContext(ctx, `select count(*) from tbl_account where fld_account='alice'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("account count = %d", count)
	}
}

func TestTransactionRollback(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), OpenOptions{Path: filepath.Join(t.TempDir(), "tx.db3"), CreateIfMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	wantErr := sql.ErrNoRows
	err = store.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `insert into tbl_account(fld_account,fld_password) values('alice','pw')`); err != nil {
			return err
		}
		return wantErr
	})
	if err != wantErr {
		t.Fatalf("tx error = %v", err)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, `select count(*) from tbl_account where fld_account='alice'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back account count = %d", count)
	}
}

func TestOpenMissingDatabase(t *testing.T) {
	t.Parallel()

	_, err := Open(context.Background(), OpenOptions{Path: filepath.Join(t.TempDir(), "missing.db3")})
	if err == nil {
		t.Fatal("expected missing database error")
	}
}
