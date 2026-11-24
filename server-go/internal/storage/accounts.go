package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrAccountExists   = errors.New("account already exists")
	ErrAccountNotFound = errors.New("account not found")
)

// Account is one row of tbl_account.
type Account struct {
	Name     string
	Password string
}

// AccountRepository owns account persistence.
type AccountRepository struct {
	store *Store
}

func NewAccountRepository(store *Store) *AccountRepository {
	return &AccountRepository{store: store}
}

func validateLegacyID(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("value is empty")
	}
	if len([]byte(value)) > 64 {
		return fmt.Errorf("value exceeds legacy 64-byte limit: %d", len([]byte(value)))
	}
	return nil
}

// Create inserts one account using the legacy plain-text credential format.
func (r *AccountRepository) Create(ctx context.Context, name, password string) error {
	if err := validateLegacyID(name); err != nil {
		return fmt.Errorf("account name: %w", err)
	}
	if err := validateLegacyID(password); err != nil {
		return fmt.Errorf("password: %w", err)
	}
	_, err := r.store.db.ExecContext(ctx, `insert into tbl_account(fld_account, fld_password) values(?, ?)`, name, password)
	if err != nil {
		if exists, queryErr := r.Exists(ctx, name); queryErr == nil && exists {
			return ErrAccountExists
		}
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

func (r *AccountRepository) Exists(ctx context.Context, name string) (bool, error) {
	var count int
	if err := r.store.db.QueryRowContext(ctx, `select count(*) from tbl_account where fld_account = ?`, name).Scan(&count); err != nil {
		return false, fmt.Errorf("check account: %w", err)
	}
	return count != 0, nil
}

func (r *AccountRepository) Get(ctx context.Context, name string) (Account, error) {
	var account Account
	err := r.store.db.QueryRowContext(ctx, `select fld_account, fld_password from tbl_account where fld_account = ?`, name).
		Scan(&account.Name, &account.Password)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("get account: %w", err)
	}
	return account, nil
}

// Authenticate performs the legacy exact-match password comparison.
func (r *AccountRepository) Authenticate(ctx context.Context, name, password string) (bool, error) {
	account, err := r.Get(ctx, name)
	if errors.Is(err, ErrAccountNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return account.Password == password, nil
}

func (r *AccountRepository) ChangePassword(ctx context.Context, name, password string) error {
	if err := validateLegacyID(password); err != nil {
		return fmt.Errorf("password: %w", err)
	}
	result, err := r.store.db.ExecContext(ctx, `update tbl_account set fld_password = ? where fld_account = ?`, password, name)
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("change password rows: %w", err)
	}
	if affected == 0 {
		return ErrAccountNotFound
	}
	return nil
}

func (r *AccountRepository) Delete(ctx context.Context, name string) error {
	result, err := r.store.db.ExecContext(ctx, `delete from tbl_account where fld_account = ?`, name)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete account rows: %w", err)
	}
	if affected == 0 {
		return ErrAccountNotFound
	}
	return nil
}
