package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

var (
	ErrInvalidItem = errors.New("invalid item")
	ErrBeltSlot    = errors.New("invalid belt slot")
	ErrWearSlot    = errors.New("invalid wear slot")
)

const (
	beltSlotCount = 6
	wearSlotMin   = 1
	wearSlotMax   = 11
)

// ItemRepository owns inventory, wear, belt, secured-item, magic and player-config storage.
type ItemRepository struct {
	store *Store
}

func NewItemRepository(store *Store) *ItemRepository {
	return &ItemRepository{store: store}
}

func validateItem(item protocol.SDItem, requireSeqID bool) error {
	if item.ItemID == 0 {
		return fmt.Errorf("%w: item id is zero", ErrInvalidItem)
	}
	if requireSeqID && item.SeqID == 0 {
		return fmt.Errorf("%w: seq id is zero", ErrInvalidItem)
	}
	if item.Count == 0 {
		return fmt.Errorf("%w: count is zero", ErrInvalidItem)
	}
	return nil
}

func (r *ItemRepository) LoadInventory(ctx context.Context, dbid uint32) ([]protocol.SDItem, error) {
	rows, err := r.store.db.QueryContext(ctx, `
select fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist
from tbl_inventory where fld_dbid = ? order by fld_itemid, fld_seqid`, dbid)
	if err != nil {
		return nil, fmt.Errorf("load inventory: %w", err)
	}
	defer rows.Close()
	var result []protocol.SDItem
	for rows.Next() {
		item, err := scanStoredItem(rows, true)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inventory: %w", err)
	}
	return result, nil
}

func scanStoredItem(scanner interface{ Scan(...any) error }, withSeqID bool) (protocol.SDItem, error) {
	var item protocol.SDItem
	var blob []byte
	var err error
	if withSeqID {
		err = scanner.Scan(&item.ItemID, &item.SeqID, &item.Count, &item.Duration[0], &item.Duration[1], &blob)
	} else {
		item.SeqID = 0
		err = scanner.Scan(&item.ItemID, &item.Count, &item.Duration[0], &item.Duration[1], &blob)
	}
	if err != nil {
		return protocol.SDItem{}, err
	}
	attributes, err := unmarshalExtAttrList(blob)
	if err != nil {
		return protocol.SDItem{}, fmt.Errorf("decode item ext attrs: %w", err)
	}
	item.ExtAttrList = attributes
	return item, nil
}

func (r *ItemRepository) PutInventoryItem(ctx context.Context, dbid uint32, item protocol.SDItem) error {
	if err := validateItem(item, true); err != nil {
		return err
	}
	blob, err := marshalExtAttrList(item.ExtAttrList)
	if err != nil {
		return fmt.Errorf("encode item ext attrs: %w", err)
	}
	_, err = r.store.db.ExecContext(ctx, `
replace into tbl_inventory
(fld_dbid, fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist)
values(?, ?, ?, ?, ?, ?, ?)`,
		dbid, item.ItemID, item.SeqID, item.Count, item.Duration[0], item.Duration[1], blob)
	if err != nil {
		return fmt.Errorf("put inventory item: %w", err)
	}
	return nil
}

// StoreInventory atomically replaces the complete inventory.
func (r *ItemRepository) StoreInventory(ctx context.Context, dbid uint32, items []protocol.SDItem) error {
	seen := make(map[uint64]bool, len(items))
	for _, item := range items {
		if err := validateItem(item, true); err != nil {
			return err
		}
		key := item.ItemIDSeq()
		if seen[key] {
			return fmt.Errorf("%w: duplicate item %016x", ErrInvalidItem, key)
		}
		seen[key] = true
	}
	return r.store.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `delete from tbl_inventory where fld_dbid = ?`, dbid); err != nil {
			return fmt.Errorf("clear inventory: %w", err)
		}
		for _, item := range items {
			blob, err := marshalExtAttrList(item.ExtAttrList)
			if err != nil {
				return fmt.Errorf("encode item ext attrs: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
insert into tbl_inventory
(fld_dbid, fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist)
values(?, ?, ?, ?, ?, ?, ?)`,
				dbid, item.ItemID, item.SeqID, item.Count, item.Duration[0], item.Duration[1], blob); err != nil {
				return fmt.Errorf("insert inventory item: %w", err)
			}
		}
		return nil
	})
}

func (r *ItemRepository) RemoveInventoryItem(ctx context.Context, dbid, itemID, seqID uint32) (bool, error) {
	result, err := r.store.db.ExecContext(ctx, `delete from tbl_inventory where fld_dbid = ? and fld_itemid = ? and fld_seqid = ?`, dbid, itemID, seqID)
	if err != nil {
		return false, fmt.Errorf("remove inventory item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("remove inventory item rows: %w", err)
	}
	return affected != 0, nil
}

func (r *ItemRepository) LoadSecuredItems(ctx context.Context, dbid uint32) ([]protocol.SDItem, error) {
	rows, err := r.store.db.QueryContext(ctx, `
select fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist
from tbl_secureditemlist where fld_dbid = ? order by fld_itemid, fld_seqid`, dbid)
	if err != nil {
		return nil, fmt.Errorf("load secured items: %w", err)
	}
	defer rows.Close()
	var result []protocol.SDItem
	for rows.Next() {
		item, err := scanStoredItem(rows, true)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate secured items: %w", err)
	}
	return result, nil
}

func (r *ItemRepository) PutSecuredItem(ctx context.Context, dbid uint32, item protocol.SDItem) error {
	if err := validateItem(item, true); err != nil {
		return err
	}
	blob, err := marshalExtAttrList(item.ExtAttrList)
	if err != nil {
		return fmt.Errorf("encode secured item ext attrs: %w", err)
	}
	_, err = r.store.db.ExecContext(ctx, `
replace into tbl_secureditemlist
(fld_dbid, fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist)
values(?, ?, ?, ?, ?, ?, ?)`,
		dbid, item.ItemID, item.SeqID, item.Count, item.Duration[0], item.Duration[1], blob)
	if err != nil {
		return fmt.Errorf("put secured item: %w", err)
	}
	return nil
}

func (r *ItemRepository) TakeSecuredItem(ctx context.Context, dbid, itemID, seqID uint32) (protocol.SDItem, bool, error) {
	row := r.store.db.QueryRowContext(ctx, `
delete from tbl_secureditemlist
where fld_dbid = ? and fld_itemid = ? and fld_seqid = ?
returning fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist`,
		dbid, itemID, seqID)
	item, err := scanStoredItem(row, true)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.SDItem{}, false, nil
	}
	if err != nil {
		return protocol.SDItem{}, false, fmt.Errorf("take secured item: %w", err)
	}
	return item, true, nil
}

func (r *ItemRepository) LoadWear(ctx context.Context, dbid uint32) (protocol.SDWear, error) {
	rows, err := r.store.db.QueryContext(ctx, `
select fld_wear, fld_itemid, fld_count, fld_duration, fld_maxduration, fld_extattrlist
from tbl_wear where fld_dbid = ? order by fld_wear`, dbid)
	if err != nil {
		return protocol.SDWear{}, fmt.Errorf("load wear: %w", err)
	}
	defer rows.Close()
	result := protocol.SDWear{List: make(map[int32]protocol.SDItem)}
	for rows.Next() {
		var slot int32
		var item protocol.SDItem
		var blob []byte
		if err := rows.Scan(&slot, &item.ItemID, &item.Count, &item.Duration[0], &item.Duration[1], &blob); err != nil {
			return protocol.SDWear{}, fmt.Errorf("scan wear: %w", err)
		}
		if slot < wearSlotMin || slot > wearSlotMax {
			return protocol.SDWear{}, fmt.Errorf("%w: %d", ErrWearSlot, slot)
		}
		attributes, err := unmarshalExtAttrList(blob)
		if err != nil {
			return protocol.SDWear{}, fmt.Errorf("decode wear ext attrs: %w", err)
		}
		item.SeqID = 0
		item.ExtAttrList = attributes
		result.List[slot] = item
	}
	if err := rows.Err(); err != nil {
		return protocol.SDWear{}, fmt.Errorf("iterate wear: %w", err)
	}
	if len(result.List) == 0 {
		result.List = nil
	}
	return result, nil
}

func (r *ItemRepository) PutWearItem(ctx context.Context, dbid uint32, slot int32, item protocol.SDItem) error {
	if slot < wearSlotMin || slot > wearSlotMax {
		return fmt.Errorf("%w: %d", ErrWearSlot, slot)
	}
	if err := validateItem(item, false); err != nil {
		return err
	}
	blob, err := marshalExtAttrList(item.ExtAttrList)
	if err != nil {
		return fmt.Errorf("encode wear ext attrs: %w", err)
	}
	_, err = r.store.db.ExecContext(ctx, `
replace into tbl_wear
(fld_dbid, fld_wear, fld_itemid, fld_count, fld_duration, fld_maxduration, fld_extattrlist)
values(?, ?, ?, ?, ?, ?, ?)`,
		dbid, slot, item.ItemID, item.Count, item.Duration[0], item.Duration[1], blob)
	if err != nil {
		return fmt.Errorf("put wear item: %w", err)
	}
	return nil
}

func (r *ItemRepository) RemoveWearItem(ctx context.Context, dbid uint32, slot int32) (bool, error) {
	if slot < wearSlotMin || slot > wearSlotMax {
		return false, fmt.Errorf("%w: %d", ErrWearSlot, slot)
	}
	result, err := r.store.db.ExecContext(ctx, `delete from tbl_wear where fld_dbid = ? and fld_wear = ?`, dbid, slot)
	if err != nil {
		return false, fmt.Errorf("remove wear item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func (r *ItemRepository) LoadBelt(ctx context.Context, dbid uint32) (protocol.SDBelt, error) {
	rows, err := r.store.db.QueryContext(ctx, `select fld_belt, fld_itemid, fld_count from tbl_belt where fld_dbid = ? order by fld_belt`, dbid)
	if err != nil {
		return protocol.SDBelt{}, fmt.Errorf("load belt: %w", err)
	}
	defer rows.Close()
	var result protocol.SDBelt
	for rows.Next() {
		var slot int32
		var item protocol.SDItem
		if err := rows.Scan(&slot, &item.ItemID, &item.Count); err != nil {
			return protocol.SDBelt{}, fmt.Errorf("scan belt: %w", err)
		}
		if slot < 0 || slot >= beltSlotCount {
			return protocol.SDBelt{}, fmt.Errorf("%w: %d", ErrBeltSlot, slot)
		}
		item.SeqID = 0
		result.List[slot] = item
	}
	if err := rows.Err(); err != nil {
		return protocol.SDBelt{}, fmt.Errorf("iterate belt: %w", err)
	}
	return result, nil
}

func (r *ItemRepository) PutBeltItem(ctx context.Context, dbid uint32, slot int32, item protocol.SDItem) error {
	if slot < 0 || slot >= beltSlotCount {
		return fmt.Errorf("%w: %d", ErrBeltSlot, slot)
	}
	if err := validateItem(item, false); err != nil {
		return err
	}
	_, err := r.store.db.ExecContext(ctx, `replace into tbl_belt(fld_dbid, fld_belt, fld_itemid, fld_count) values(?, ?, ?, ?)`, dbid, slot, item.ItemID, item.Count)
	if err != nil {
		return fmt.Errorf("put belt item: %w", err)
	}
	return nil
}

func (r *ItemRepository) RemoveBeltItem(ctx context.Context, dbid uint32, slot int32) (bool, error) {
	if slot < 0 || slot >= beltSlotCount {
		return false, fmt.Errorf("%w: %d", ErrBeltSlot, slot)
	}
	result, err := r.store.db.ExecContext(ctx, `delete from tbl_belt where fld_dbid = ? and fld_belt = ?`, dbid, slot)
	if err != nil {
		return false, fmt.Errorf("remove belt item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func (r *ItemRepository) LoadLearnedMagic(ctx context.Context, dbid uint32) ([]protocol.SDLearnedMagic, error) {
	rows, err := r.store.db.QueryContext(ctx, `select fld_magicid, fld_exp from tbl_learnedmagiclist where fld_dbid = ? order by fld_magicid`, dbid)
	if err != nil {
		return nil, fmt.Errorf("load learned magic: %w", err)
	}
	defer rows.Close()
	var result []protocol.SDLearnedMagic
	for rows.Next() {
		var magic protocol.SDLearnedMagic
		if err := rows.Scan(&magic.MagicID, &magic.Exp); err != nil {
			return nil, fmt.Errorf("scan learned magic: %w", err)
		}
		result = append(result, magic)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate learned magic: %w", err)
	}
	return result, nil
}

func (r *ItemRepository) LearnMagic(ctx context.Context, dbid, magicID uint32) error {
	_, err := r.store.db.ExecContext(ctx, `insert into tbl_learnedmagiclist(fld_dbid, fld_magicid) values(?, ?)`, dbid, magicID)
	if err != nil {
		return fmt.Errorf("learn magic: %w", err)
	}
	return nil
}

func (r *ItemRepository) AddMagicExp(ctx context.Context, dbid, magicID uint32, exp uint64) error {
	if exp == 0 {
		return nil
	}
	_, err := r.store.db.ExecContext(ctx, `update tbl_learnedmagiclist set fld_exp = fld_exp + ? where fld_dbid = ? and fld_magicid = ?`, exp, dbid, magicID)
	if err != nil {
		return fmt.Errorf("add magic exp: %w", err)
	}
	return nil
}

func (r *ItemRepository) LoadPlayerConfig(ctx context.Context, dbid uint32) (protocol.SDPlayerConfig, error) {
	var result protocol.SDPlayerConfig
	var magicBlob, runtimeBlob []byte
	err := r.store.db.QueryRowContext(ctx, `select fld_magickeylist, fld_runtimeconfig from tbl_playerconfig where fld_dbid = ?`, dbid).
		Scan(&magicBlob, &runtimeBlob)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return protocol.SDPlayerConfig{}, fmt.Errorf("load player config: %w", err)
	}
	if len(magicBlob) != 0 {
		if err := UnmarshalCerealBlob(magicBlob, &result.MagicKeyList); err != nil {
			return protocol.SDPlayerConfig{}, fmt.Errorf("decode magic keys: %w", err)
		}
	}
	if len(runtimeBlob) != 0 {
		if err := UnmarshalCerealBlob(runtimeBlob, &result.RuntimeConfig); err != nil {
			return protocol.SDPlayerConfig{}, fmt.Errorf("decode runtime config: %w", err)
		}
	}
	return result, nil
}

func (r *ItemRepository) UpdateMagicKeyList(ctx context.Context, dbid uint32, keys protocol.SDMagicKeyList) error {
	blob, err := MarshalCerealBlob(keys)
	if err != nil {
		return fmt.Errorf("encode magic keys: %w", err)
	}
	_, err = r.store.db.ExecContext(ctx, `
insert into tbl_playerconfig(fld_dbid, fld_magickeylist) values(?, ?)
on conflict(fld_dbid) do update set fld_magickeylist = excluded.fld_magickeylist`, dbid, blob)
	if err != nil {
		return fmt.Errorf("update magic keys: %w", err)
	}
	return nil
}

func (r *ItemRepository) UpdateRuntimeConfig(ctx context.Context, dbid uint32, config protocol.SDRuntimeConfig) error {
	blob, err := MarshalCerealBlob(config)
	if err != nil {
		return fmt.Errorf("encode runtime config: %w", err)
	}
	_, err = r.store.db.ExecContext(ctx, `
insert into tbl_playerconfig(fld_dbid, fld_runtimeconfig) values(?, ?)
on conflict(fld_dbid) do update set fld_runtimeconfig = excluded.fld_runtimeconfig`, dbid, blob)
	if err != nil {
		return fmt.Errorf("update runtime config: %w", err)
	}
	return nil
}
