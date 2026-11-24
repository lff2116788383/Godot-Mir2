package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

var ErrDirectTradeCommit = errors.New("direct trade commit failed")

// TradeRepository owns the atomic direct-trade commit.
type TradeRepository struct {
	store *Store
}

func NewTradeRepository(store *Store) *TradeRepository { return &TradeRepository{store: store} }

type tradeInventoryQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadTradeInventory(ctx context.Context, queryer tradeInventoryQueryer, dbid uint32) ([]protocol.SDItem, error) {
	rows, err := queryer.QueryContext(ctx, `
select fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist
from tbl_inventory where fld_dbid = ? order by fld_itemid, fld_seqid`, dbid)
	if err != nil {
		return nil, err
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
	return result, rows.Err()
}

func removeTradeItems(inventory []protocol.SDItem, offer []protocol.SDItem) ([]protocol.SDItem, bool) {
	result := append([]protocol.SDItem(nil), inventory...)
	for _, offered := range offer {
		found := -1
		for i, item := range result {
			if item.ItemID == offered.ItemID && item.SeqID == offered.SeqID {
				found = i
				break
			}
		}
		if found < 0 || offered.Count == 0 || result[found].Count < offered.Count {
			return nil, false
		}
		if result[found].Count == offered.Count {
			result = append(result[:found], result[found+1:]...)
		} else {
			result[found].Count -= offered.Count
		}
	}
	return result, true
}

func addTradeItems(inventory []protocol.SDItem, offer []protocol.SDItem) []protocol.SDItem {
	result := append([]protocol.SDItem(nil), inventory...)
	for _, item := range offer {
		used := make(map[uint32]bool)
		for _, existing := range result {
			if existing.ItemID == item.ItemID {
				used[existing.SeqID] = true
			}
		}
		if used[item.SeqID] || item.SeqID == 0 {
			for seq := uint32(1); ; seq++ {
				if !used[seq] {
					item.SeqID = seq
					break
				}
			}
		}
		result = append(result, item)
	}
	return result
}

func storeTradeInventory(ctx context.Context, tx *sql.Tx, dbid uint32, inventory []protocol.SDItem) error {
	if _, err := tx.ExecContext(ctx, `delete from tbl_inventory where fld_dbid = ?`, dbid); err != nil {
		return err
	}
	for _, item := range inventory {
		if err := validateItem(item, true); err != nil {
			return err
		}
		blob, err := marshalExtAttrList(item.ExtAttrList)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
insert into tbl_inventory(fld_dbid, fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist)
values(?, ?, ?, ?, ?, ?, ?)`, dbid, item.ItemID, item.SeqID, item.Count, item.Duration[0], item.Duration[1], blob); err != nil {
			return err
		}
	}
	return nil
}

// Commit atomically exchanges gold and inventory items between two players.
// Offer.UID is copied into the result for wire compatibility; DBIDs are passed
// separately because uid-to-dbid mapping belongs to the actor/world layer.
func (r *TradeRepository) Commit(
	ctx context.Context,
	dbid0, dbid1 uint32,
	offer0, offer1 protocol.SDDirectTradeOffer,
) ([2]protocol.SDDirectTradeResult, error) {
	var result [2]protocol.SDDirectTradeResult
	if dbid0 == 0 || dbid1 == 0 || dbid0 == dbid1 || !offer0.Locked || !offer1.Locked || !offer0.Confirmed || !offer1.Confirmed {
		return result, ErrDirectTradeCommit
	}
	err := r.store.InTx(ctx, func(tx *sql.Tx) error {
		var gold0, gold1 int64
		if err := tx.QueryRowContext(ctx, `select fld_gold from tbl_char where fld_dbid = ?`, dbid0).Scan(&gold0); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `select fld_gold from tbl_char where fld_dbid = ?`, dbid1).Scan(&gold1); err != nil {
			return err
		}
		if gold0 < 0 || gold1 < 0 || uint64(gold0) < offer0.Gold || uint64(gold1) < offer1.Gold {
			return ErrDirectTradeCommit
		}
		final0 := uint64(gold0) - offer0.Gold + offer1.Gold
		final1 := uint64(gold1) - offer1.Gold + offer0.Gold
		if final0 > math.MaxInt64 || final1 > math.MaxInt64 {
			return ErrDirectTradeCommit
		}
		if _, err := tx.ExecContext(ctx, `update tbl_char set fld_gold = ? where fld_dbid = ?`, final0, dbid0); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update tbl_char set fld_gold = ? where fld_dbid = ?`, final1, dbid1); err != nil {
			return err
		}

		inventory0, err := loadTradeInventory(ctx, tx, dbid0)
		if err != nil {
			return err
		}
		inventory1, err := loadTradeInventory(ctx, tx, dbid1)
		if err != nil {
			return err
		}
		inventory0, ok := removeTradeItems(inventory0, offer0.ItemList)
		if !ok {
			return ErrDirectTradeCommit
		}
		inventory1, ok = removeTradeItems(inventory1, offer1.ItemList)
		if !ok {
			return ErrDirectTradeCommit
		}
		inventory0 = addTradeItems(inventory0, offer1.ItemList)
		inventory1 = addTradeItems(inventory1, offer0.ItemList)
		if err := storeTradeInventory(ctx, tx, dbid0, inventory0); err != nil {
			return err
		}
		if err := storeTradeInventory(ctx, tx, dbid1, inventory1); err != nil {
			return err
		}

		result[0] = protocol.SDDirectTradeResult{UID: offer0.UID, Gold: final0, Inventory: protocol.SDInventory{ItemList: inventory0}}
		result[1] = protocol.SDDirectTradeResult{UID: offer1.UID, Gold: final1, Inventory: protocol.SDInventory{ItemList: inventory1}}
		return nil
	})
	if err != nil {
		return [2]protocol.SDDirectTradeResult{}, fmt.Errorf("commit direct trade: %w", err)
	}
	return result, nil
}
