package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// GoldRepository owns the positive signed 64-bit gold field.
type GoldRepository struct {
	store *Store
}

func NewGoldRepository(store *Store) *GoldRepository { return &GoldRepository{store: store} }

func validateGold(value uint64) error {
	if value > math.MaxInt64 {
		return fmt.Errorf("gold %d exceeds signed sqlite range", value)
	}
	return nil
}

func (r *GoldRepository) Load(ctx context.Context, dbid uint32) (uint64, error) {
	var gold int64
	err := r.store.db.QueryRowContext(ctx, `select fld_gold from tbl_char where fld_dbid = ?`, dbid).Scan(&gold)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrCharacterNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("load gold: %w", err)
	}
	if gold < 0 {
		return 0, fmt.Errorf("negative persisted gold %d", gold)
	}
	return uint64(gold), nil
}

func (r *GoldRepository) Set(ctx context.Context, dbid uint32, gold uint64) error {
	if err := validateGold(gold); err != nil {
		return err
	}
	result, err := r.store.db.ExecContext(ctx, `update tbl_char set fld_gold = ? where fld_dbid = ?`, gold, dbid)
	if err != nil {
		return fmt.Errorf("set gold: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *GoldRepository) Remove(ctx context.Context, dbid uint32, amount uint64) (uint64, bool, error) {
	if amount == 0 {
		remaining, err := r.Load(ctx, dbid)
		return remaining, err == nil, err
	}
	if err := validateGold(amount); err != nil {
		return 0, false, err
	}
	var remaining int64
	err := r.store.db.QueryRowContext(ctx, `
update tbl_char set fld_gold = fld_gold - ?
where fld_dbid = ? and fld_gold >= ?
returning fld_gold`, amount, dbid, amount).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("remove gold: %w", err)
	}
	return uint64(remaining), true, nil
}

func (r *GoldRepository) Add(ctx context.Context, dbid uint32, amount uint64) (uint64, error) {
	if amount == 0 {
		return r.Load(ctx, dbid)
	}
	if err := validateGold(amount); err != nil {
		return 0, err
	}
	limit := uint64(math.MaxInt64) - amount
	var total int64
	err := r.store.db.QueryRowContext(ctx, `
update tbl_char set fld_gold = fld_gold + ?
where fld_dbid = ? and fld_gold <= ?
returning fld_gold`, amount, dbid, limit).Scan(&total)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("add gold overflow or missing character")
	}
	if err != nil {
		return 0, fmt.Errorf("add gold: %w", err)
	}
	return uint64(total), nil
}

// CharacterVarRepository owns Lua-variable BLOBs in tbl_charvarlist.
type CharacterVarRepository struct {
	store *Store
}

func NewCharacterVarRepository(store *Store) *CharacterVarRepository {
	return &CharacterVarRepository{store: store}
}

func validateVarName(name string) error {
	if name == "" {
		return errors.New("variable name is empty")
	}
	if len([]byte(name)) > 1024 {
		return fmt.Errorf("variable name exceeds 1024 bytes")
	}
	return nil
}

func (r *CharacterVarRepository) Load(ctx context.Context, dbid uint32, name string) (protocol.LuaVar, bool, error) {
	if err := validateVarName(name); err != nil {
		return protocol.LuaVar{}, false, err
	}
	var blob []byte
	err := r.store.db.QueryRowContext(ctx, `select fld_value from tbl_charvarlist where fld_dbid = ? and fld_var = ? and fld_value is not null`, dbid, name).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.NewLuaNilVar(), false, nil
	}
	if err != nil {
		return protocol.LuaVar{}, false, fmt.Errorf("load character var: %w", err)
	}
	var value protocol.LuaVar
	if err := UnmarshalCerealBlob(blob, &value); err != nil {
		return protocol.LuaVar{}, false, fmt.Errorf("decode character var: %w", err)
	}
	return value, true, nil
}

func (r *CharacterVarRepository) Set(ctx context.Context, dbid uint32, name string, value protocol.LuaVar) error {
	if err := validateVarName(name); err != nil {
		return err
	}
	if value.Kind == protocol.LuaVarNil {
		return r.Delete(ctx, dbid, name)
	}
	blob, err := MarshalCerealBlob(value)
	if err != nil {
		return fmt.Errorf("encode character var: %w", err)
	}
	_, err = r.store.db.ExecContext(ctx, `
insert into tbl_charvarlist(fld_dbid, fld_var, fld_value) values(?, ?, ?)
on conflict(fld_dbid, fld_var) do update set fld_value = excluded.fld_value`, dbid, name, blob)
	if err != nil {
		return fmt.Errorf("set character var: %w", err)
	}
	return nil
}

func (r *CharacterVarRepository) Delete(ctx context.Context, dbid uint32, name string) error {
	if err := validateVarName(name); err != nil {
		return err
	}
	if _, err := r.store.db.ExecContext(ctx, `delete from tbl_charvarlist where fld_dbid = ? and fld_var = ?`, dbid, name); err != nil {
		return fmt.Errorf("delete character var: %w", err)
	}
	return nil
}
