package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrCharacterExists   = errors.New("character already exists")
	ErrCharacterNotFound = errors.New("character not found")
)

// Character mirrors the row shape of tbl_char.
type Character struct {
	DBID      uint32
	Account   string
	Name      string
	Gender    uint8
	Job       uint8
	Map       uint32
	MapX      int32
	MapY      int32
	HP        int32
	MP        int32
	Exp       uint64
	Gold      uint64
	Hair      int32
	HairColor int32
	PKPoint   int32
	ViewRange int32
}

// CreateCharacterParams contains the explicit columns used when creating a character.
type CreateCharacterParams struct {
	Account string
	Name    string
	Gender  uint8
	Job     uint8
	Map     uint32
	MapX    int32
	MapY    int32
}

// CharacterRepository owns character persistence.
type CharacterRepository struct {
	store *Store
}

func NewCharacterRepository(store *Store) *CharacterRepository {
	return &CharacterRepository{store: store}
}

func (r *CharacterRepository) Create(ctx context.Context, params CreateCharacterParams) (Character, error) {
	if err := validateLegacyID(params.Account); err != nil {
		return Character{}, fmt.Errorf("account: %w", err)
	}
	if err := validateLegacyID(params.Name); err != nil {
		return Character{}, fmt.Errorf("character name: %w", err)
	}
	if params.Gender > 1 {
		return Character{}, fmt.Errorf("invalid gender %d", params.Gender)
	}
	if !validJob(params.Job) {
		return Character{}, fmt.Errorf("invalid job %d", params.Job)
	}
	row := r.store.db.QueryRowContext(ctx, `
insert into tbl_char(fld_account, fld_name, fld_gender, fld_job, fld_map, fld_mapx, fld_mapy)
values(?, ?, ?, ?, ?, ?, ?)
returning fld_dbid, fld_account, fld_name, fld_gender, fld_job, fld_map, fld_mapx, fld_mapy,
          fld_hp, fld_mp, fld_exp, fld_gold, fld_hair, fld_haircolor, fld_pkpoint, fld_viewr`,
		params.Account, params.Name, params.Gender, params.Job, params.Map, params.MapX, params.MapY)
	character, err := scanCharacter(row)
	if err != nil {
		if exists, queryErr := r.ExistsByName(ctx, params.Name); queryErr == nil && exists {
			return Character{}, ErrCharacterExists
		}
		return Character{}, fmt.Errorf("create character: %w", err)
	}
	return character, nil
}

func validJob(job uint8) bool {
	return job != 0 && job&^uint8(7) == 0
}

func scanCharacter(scanner interface{ Scan(...any) error }) (Character, error) {
	var character Character
	err := scanner.Scan(
		&character.DBID,
		&character.Account,
		&character.Name,
		&character.Gender,
		&character.Job,
		&character.Map,
		&character.MapX,
		&character.MapY,
		&character.HP,
		&character.MP,
		&character.Exp,
		&character.Gold,
		&character.Hair,
		&character.HairColor,
		&character.PKPoint,
		&character.ViewRange,
	)
	return character, err
}

const characterColumns = `fld_dbid, fld_account, fld_name, fld_gender, fld_job, fld_map, fld_mapx, fld_mapy,
fld_hp, fld_mp, fld_exp, fld_gold, fld_hair, fld_haircolor, fld_pkpoint, fld_viewr`

func (r *CharacterRepository) GetByDBID(ctx context.Context, dbid uint32) (Character, error) {
	row := r.store.db.QueryRowContext(ctx, `select `+characterColumns+` from tbl_char where fld_dbid = ?`, dbid)
	character, err := scanCharacter(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Character{}, ErrCharacterNotFound
	}
	if err != nil {
		return Character{}, fmt.Errorf("get character by dbid: %w", err)
	}
	return character, nil
}

func (r *CharacterRepository) GetByAccount(ctx context.Context, account string) (Character, error) {
	row := r.store.db.QueryRowContext(ctx, `select `+characterColumns+` from tbl_char where fld_account = ?`, account)
	character, err := scanCharacter(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Character{}, ErrCharacterNotFound
	}
	if err != nil {
		return Character{}, fmt.Errorf("get character by account: %w", err)
	}
	return character, nil
}

func (r *CharacterRepository) GetByName(ctx context.Context, name string) (Character, error) {
	row := r.store.db.QueryRowContext(ctx, `select `+characterColumns+` from tbl_char where fld_name = ?`, name)
	character, err := scanCharacter(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Character{}, ErrCharacterNotFound
	}
	if err != nil {
		return Character{}, fmt.Errorf("get character by name: %w", err)
	}
	return character, nil
}

func (r *CharacterRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int
	if err := r.store.db.QueryRowContext(ctx, `select count(*) from tbl_char where fld_name = ?`, name).Scan(&count); err != nil {
		return false, fmt.Errorf("check character name: %w", err)
	}
	return count != 0, nil
}

func (r *CharacterRepository) ListByAccount(ctx context.Context, account string) ([]Character, error) {
	rows, err := r.store.db.QueryContext(ctx, `select `+characterColumns+` from tbl_char where fld_account = ? order by fld_dbid`, account)
	if err != nil {
		return nil, fmt.Errorf("list characters: %w", err)
	}
	defer rows.Close()
	var result []Character
	for rows.Next() {
		character, err := scanCharacter(rows)
		if err != nil {
			return nil, fmt.Errorf("scan character: %w", err)
		}
		result = append(result, character)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate characters: %w", err)
	}
	return result, nil
}

func (r *CharacterRepository) Delete(ctx context.Context, dbid uint32) error {
	result, err := r.store.db.ExecContext(ctx, `delete from tbl_char where fld_dbid = ?`, dbid)
	if err != nil {
		return fmt.Errorf("delete character: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete character rows: %w", err)
	}
	if affected == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *CharacterRepository) UpdatePosition(ctx context.Context, dbid, mapID uint32, x, y int32) error {
	return r.execUpdate(ctx, `update tbl_char set fld_map = ?, fld_mapx = ?, fld_mapy = ? where fld_dbid = ?`, mapID, x, y, dbid)
}

func (r *CharacterRepository) UpdateHealth(ctx context.Context, dbid uint32, hp, mp int32) error {
	return r.execUpdate(ctx, `update tbl_char set fld_hp = ?, fld_mp = ? where fld_dbid = ?`, hp, mp, dbid)
}

func (r *CharacterRepository) UpdateExp(ctx context.Context, dbid uint32, exp uint64) error {
	return r.execUpdate(ctx, `update tbl_char set fld_exp = ? where fld_dbid = ?`, exp, dbid)
}

func (r *CharacterRepository) UpdateGold(ctx context.Context, dbid uint32, gold uint64) error {
	return r.execUpdate(ctx, `update tbl_char set fld_gold = ? where fld_dbid = ?`, gold, dbid)
}

func (r *CharacterRepository) UpdatePKPoint(ctx context.Context, dbid uint32, pkPoint int32) error {
	return r.execUpdate(ctx, `update tbl_char set fld_pkpoint = ? where fld_dbid = ?`, pkPoint, dbid)
}

func (r *CharacterRepository) UpdateViewRange(ctx context.Context, dbid uint32, viewRange int32) error {
	return r.execUpdate(ctx, `update tbl_char set fld_viewr = ? where fld_dbid = ?`, viewRange, dbid)
}

func (r *CharacterRepository) UpdateAppearance(ctx context.Context, dbid uint32, hair, hairColor int32) error {
	return r.execUpdate(ctx, `update tbl_char set fld_hair = ?, fld_haircolor = ? where fld_dbid = ?`, hair, hairColor, dbid)
}

func (r *CharacterRepository) execUpdate(ctx context.Context, query string, args ...any) error {
	result, err := r.store.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
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
