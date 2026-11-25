package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

const (
	AuctionCategoryAll = 1
	AuctionCategoryEnd = 12

	auctionLifetimeSeconds = int64(7 * 24 * 60 * 60)
	auctionNoteMaxBytes    = 128
	stallNameMaxBytes      = 48
	stallMaxItems          = 36
)

var (
	ErrAuctionNotFound = errors.New("auction item not found")
	ErrAuctionSoldOut  = errors.New("auction item unavailable")
	ErrStallSelling    = errors.New("stall is already selling")
	ErrStallFull       = errors.New("stall is full")
	ErrStallItemAbsent = errors.New("stall item not found")
)

// AuctionRepository owns auction listing persistence.
type AuctionRepository struct {
	store *Store
}

func NewAuctionRepository(store *Store) *AuctionRepository { return &AuctionRepository{store: store} }

func (r *AuctionRepository) Query(ctx context.Context, category int32, now int64) (protocol.SDAuctionItemList, error) {
	if category < AuctionCategoryAll || category >= AuctionCategoryEnd {
		return protocol.SDAuctionItemList{}, fmt.Errorf("invalid auction category %d", category)
	}
	if now == 0 {
		now = time.Now().Unix()
	}
	rows, err := r.store.db.QueryContext(ctx, `
select a.fld_id, a.fld_seller, a.fld_note, a.fld_itemid, a.fld_seqid, a.fld_count,
       a.fld_duration, a.fld_maxduration, a.fld_extattrlist, a.fld_price, a.fld_expiretime,
       c.fld_name, c.fld_gender, c.fld_job
from tbl_auctionitem a join tbl_char c on c.fld_dbid = a.fld_seller
where a.fld_expiretime > ? and checkAuctionCategory(a.fld_itemid, ?) = 1
order by a.fld_id`, now, category)
	if err != nil {
		return protocol.SDAuctionItemList{}, fmt.Errorf("query auction: %w", err)
	}
	defer rows.Close()

	result := protocol.SDAuctionItemList{Category: category}
	for rows.Next() {
		var item protocol.SDAuctionItem
		var sellerID uint32
		var gender uint8
		var job int32
		var blob []byte
		var expireTime int64
		if err := rows.Scan(
			&item.AuctionID, &sellerID, &item.Note,
			&item.Item.ItemID, &item.Item.SeqID, &item.Item.Count,
			&item.Item.Duration[0], &item.Item.Duration[1], &blob,
			&item.Price, &expireTime, &item.Seller.Name, &gender, &job,
		); err != nil {
			return protocol.SDAuctionItemList{}, fmt.Errorf("scan auction: %w", err)
		}
		attributes, err := unmarshalExtAttrList(blob)
		if err != nil {
			return protocol.SDAuctionItemList{}, fmt.Errorf("decode auction item ext attrs: %w", err)
		}
		item.Item.ExtAttrList = attributes
		item.Seller.ID = sellerID
		item.Seller.DespKind = protocol.SDChatPeerDespPlayer
		item.Seller.PlayerVar = protocol.SDChatPeerPlayerVar{Gender: gender != 0, Job: job}
		item.TimeLeft = uint64(expireTime - now)
		result.ItemList = append(result.ItemList, item)
	}
	if err := rows.Err(); err != nil {
		return protocol.SDAuctionItemList{}, err
	}
	return result, nil
}

func (r *AuctionRepository) Register(ctx context.Context, sellerDBID uint32, item protocol.SDItem, note string, price uint64, expireTime int64) (uint64, error) {
	if err := validateItem(item, true); err != nil {
		return 0, err
	}
	if price == 0 {
		return 0, errors.New("auction price is zero")
	}
	if len([]byte(note)) > auctionNoteMaxBytes {
		return 0, fmt.Errorf("auction note exceeds %d bytes", auctionNoteMaxBytes)
	}
	if expireTime == 0 {
		expireTime = time.Now().Unix() + auctionLifetimeSeconds
	}
	blob, err := marshalExtAttrList(item.ExtAttrList)
	if err != nil {
		return 0, err
	}
	var auctionID uint64
	err = r.store.InTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `delete from tbl_inventory where fld_dbid = ? and fld_itemid = ? and fld_seqid = ?`, sellerDBID, item.ItemID, item.SeqID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrInvalidItem
		}
		return tx.QueryRowContext(ctx, `
insert into tbl_auctionitem(fld_seller, fld_note, fld_itemid, fld_seqid, fld_count,
fld_duration, fld_maxduration, fld_extattrlist, fld_price, fld_expiretime)
values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?) returning fld_id`,
			sellerDBID, note, item.ItemID, item.SeqID, item.Count, item.Duration[0], item.Duration[1], blob, price, expireTime).Scan(&auctionID)
	})
	if err != nil {
		return 0, fmt.Errorf("register auction item: %w", err)
	}
	return auctionID, nil
}

type AuctionTakeResult struct {
	AuctionID  uint64
	SellerDBID uint32
	Item       protocol.SDItem
	Price      uint64
}

func (r *AuctionRepository) Take(ctx context.Context, auctionID uint64, now int64) (AuctionTakeResult, error) {
	if now == 0 {
		now = time.Now().Unix()
	}
	var result AuctionTakeResult
	var blob []byte
	err := r.store.db.QueryRowContext(ctx, `
delete from tbl_auctionitem where fld_id = ? and fld_expiretime > ?
returning fld_seller, fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist, fld_price`,
		auctionID, now).Scan(&result.SellerDBID, &result.Item.ItemID, &result.Item.SeqID, &result.Item.Count,
		&result.Item.Duration[0], &result.Item.Duration[1], &blob, &result.Price)
	if errors.Is(err, sql.ErrNoRows) {
		return AuctionTakeResult{}, ErrAuctionSoldOut
	}
	if err != nil {
		return AuctionTakeResult{}, fmt.Errorf("take auction item: %w", err)
	}
	result.AuctionID = auctionID
	attributes, err := unmarshalExtAttrList(blob)
	if err != nil {
		return AuctionTakeResult{}, err
	}
	result.Item.ExtAttrList = attributes
	return result, nil
}

// Reinstate 把已经 Take 走但未能成交的拍卖物品放回表里。
//
// ★ 这是**失败补偿**，用于实现原版的「事务回滚」语义。
//
//	原版 dbBuyAuctionItem 整个流程在一个事务里：
//	delete 取走 → 判自己买自己 → 扣钱 → 建投递，任一步返回 unexpected
//	都会让 transaction 析构时回滚，「物品回到拍卖行」是自动的。
//
//	Go 侧 storage 是 autocommit，所以 Take 之后必须显式放回。
//	两个补偿点：自己买自己的（OWNITEM）、钱不够（INSUFFICIENT）。
//
// 注意：放回时用 Take 拿到的原 SellerDBID / Price / Item，
// expireTime 用「一个新周期」而不是原始值 —— 因为 Take 的 returning
// 没有带 fld_expiretime。代价是补偿后物品会多挂 7 天，
// 这比「物品凭空消失」好得多（原版回滚会恢复原到期时间）。
func (r *AuctionRepository) Reinstate(ctx context.Context, taken AuctionTakeResult) error {
	if taken.SellerDBID == 0 || taken.Item.ItemID == 0 {
		return fmt.Errorf("reinstate: incomplete take result")
	}
	blob, err := marshalExtAttrList(taken.Item.ExtAttrList)
	if err != nil {
		return fmt.Errorf("reinstate extattr: %w", err)
	}
	expire := taken.Price // placeholder, overwritten below
	_ = expire
	var newExpire int64 = time.Now().Unix() + 7*24*60*60

	item := taken.Item
	_, err = r.store.db.ExecContext(ctx, `
insert into tbl_auctionitem(fld_seller, fld_note, fld_itemid, fld_seqid, fld_count,
fld_duration, fld_maxduration, fld_extattrlist, fld_price, fld_expiretime)
values(?, '', ?, ?, ?, ?, ?, ?, ?, ?)`,
		taken.SellerDBID, item.ItemID, item.SeqID, item.Count,
		item.Duration[0], item.Duration[1], blob, taken.Price, newExpire)
	if err != nil {
		return fmt.Errorf("reinstate auction item: %w", err)
	}
	return nil
}

// AuctionExpired 是一条已过期、待退回卖家的拍卖记录。
type AuctionExpired struct {
	AuctionID  uint64
	SellerDBID uint32
}

// LoadExpired 列出已过期的拍卖（fld_expiretime <= now）。
//
// ★ 为什么不能直接用 DeleteExpired：
//
//	DeleteExpired 只删不发，过期物品就永久消失了。
//	原版是「惰性过期」（所有查询都带 expiretime > now），
//	物品其实一直留在表里，玩家永远也拿不回来。
//
//	Go 侧既然要清理，就必须连带把物品退回卖家 —— 所以先查后逐条退。
func (r *AuctionRepository) LoadExpired(ctx context.Context, now int64) ([]AuctionExpired, error) {
	if now == 0 {
		now = time.Now().Unix()
	}
	rows, err := r.store.db.QueryContext(ctx, `
select fld_id, fld_seller from tbl_auctionitem
where fld_expiretime <= ? order by fld_expiretime, fld_id`, now)
	if err != nil {
		return nil, fmt.Errorf("load expired auctions: %w", err)
	}
	defer rows.Close()
	var result []AuctionExpired
	for rows.Next() {
		var record AuctionExpired
		if err := rows.Scan(&record.AuctionID, &record.SellerDBID); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

// TakeExpired 原子取走一条**已过期**的拍卖（与 Take 相反的条件）。
//
// ★ 条件写成 `fld_expiretime <= ?` 而不是 `> ?`：
//
//	这是「过期回收」专用。写成 > 的话会跟买家抢同一件物品 ——
//	买家 Take 成功、清理器也 Take 成功，物品被退回两次。
//	SQLite 的 delete ... returning 是原子的，两个 Take 只有一个能拿到行。
func (r *AuctionRepository) TakeExpired(ctx context.Context, auctionID uint64, now int64) (protocol.SDItem, bool, error) {
	if now == 0 {
		now = time.Now().Unix()
	}
	var item protocol.SDItem
	var blob []byte
	err := r.store.db.QueryRowContext(ctx, `
delete from tbl_auctionitem where fld_id = ? and fld_expiretime <= ?
returning fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist`,
		auctionID, now).Scan(&item.ItemID, &item.SeqID, &item.Count,
		&item.Duration[0], &item.Duration[1], &blob)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.SDItem{}, false, nil
	}
	if err != nil {
		return protocol.SDItem{}, false, fmt.Errorf("take expired auction: %w", err)
	}
	attributes, err := unmarshalExtAttrList(blob)
	if err != nil {
		return protocol.SDItem{}, false, err
	}
	item.ExtAttrList = attributes
	item.SeqID = 0
	return item, true, nil
}

func (r *AuctionRepository) Unregister(ctx context.Context, sellerDBID uint32, auctionID uint64) (protocol.SDItem, bool, error) {	var item protocol.SDItem
	var blob []byte
	err := r.store.db.QueryRowContext(ctx, `
delete from tbl_auctionitem where fld_id = ? and fld_seller = ?
returning fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist`,
		auctionID, sellerDBID).Scan(&item.ItemID, &item.SeqID, &item.Count, &item.Duration[0], &item.Duration[1], &blob)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.SDItem{}, false, nil
	}
	if err != nil {
		return protocol.SDItem{}, false, fmt.Errorf("unregister auction item: %w", err)
	}
	attributes, err := unmarshalExtAttrList(blob)
	if err != nil {
		return protocol.SDItem{}, false, err
	}
	item.ExtAttrList = attributes
	item.SeqID = 0
	return item, true, nil
}

func (r *AuctionRepository) DeleteExpired(ctx context.Context, now int64) (int64, error) {
	result, err := r.store.db.ExecContext(ctx, `delete from tbl_auctionitem where fld_expiretime <= ?`, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired auctions: %w", err)
	}
	return result.RowsAffected()
}

// StallRepository owns booth persistence.
type StallRepository struct {
	store *Store
}

func NewStallRepository(store *Store) *StallRepository { return &StallRepository{store: store} }

func (r *StallRepository) Load(ctx context.Context, dbid uint32) (protocol.SDStall, bool, error) {
	var stall protocol.SDStall
	err := r.store.db.QueryRowContext(ctx, `select fld_name, fld_selling from tbl_stall where fld_dbid = ?`, dbid).Scan(&stall.Name, &stall.Selling)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.SDStall{}, false, nil
	}
	if err != nil {
		return protocol.SDStall{}, false, fmt.Errorf("load stall: %w", err)
	}
	stall.DBID = dbid
	rows, err := r.store.db.QueryContext(ctx, `
select fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist, fld_price
from tbl_stallitem where fld_dbid = ? order by rowid`, dbid)
	if err != nil {
		return protocol.SDStall{}, false, fmt.Errorf("load stall items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var stallItem protocol.SDStallItem
		var blob []byte
		if err := rows.Scan(&stallItem.Item.ItemID, &stallItem.Item.SeqID, &stallItem.Item.Count,
			&stallItem.Item.Duration[0], &stallItem.Item.Duration[1], &blob, &stallItem.Price); err != nil {
			return protocol.SDStall{}, false, err
		}
		attributes, err := unmarshalExtAttrList(blob)
		if err != nil {
			return protocol.SDStall{}, false, err
		}
		stallItem.Item.ExtAttrList = attributes
		stall.ItemList = append(stall.ItemList, stallItem)
	}
	if err := rows.Err(); err != nil {
		return protocol.SDStall{}, false, err
	}
	return stall, true, nil
}

func (r *StallRepository) AddItem(ctx context.Context, dbid uint32, item protocol.SDItem, price uint64) (protocol.SDStallItem, error) {
	if err := validateItem(item, true); err != nil {
		return protocol.SDStallItem{}, err
	}
	if price == 0 {
		return protocol.SDStallItem{}, errors.New("stall price is zero")
	}
	blob, err := marshalExtAttrList(item.ExtAttrList)
	if err != nil {
		return protocol.SDStallItem{}, err
	}
	var result protocol.SDStallItem
	err = r.store.InTx(ctx, func(tx *sql.Tx) error {
		var selling bool
		if err := tx.QueryRowContext(ctx, `select fld_selling from tbl_stall where fld_dbid = ?`, dbid).Scan(&selling); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if _, err := tx.ExecContext(ctx, `insert into tbl_stall(fld_dbid) values(?)`, dbid); err != nil {
				return err
			}
		}
		if selling {
			return ErrStallSelling
		}
		var count int
		if err := tx.QueryRowContext(ctx, `select count(*) from tbl_stallitem where fld_dbid = ?`, dbid).Scan(&count); err != nil {
			return err
		}
		if count >= stallMaxItems {
			return ErrStallFull
		}
		seqID := uint32(1)
		for {
			var exists int
			if err := tx.QueryRowContext(ctx, `select count(*) from tbl_stallitem where fld_dbid = ? and fld_itemid = ? and fld_seqid = ?`, dbid, item.ItemID, seqID).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				break
			}
			seqID++
		}
		inventoryDelete, err := tx.ExecContext(ctx, `delete from tbl_inventory where fld_dbid = ? and fld_itemid = ? and fld_seqid = ?`, dbid, item.ItemID, item.SeqID)
		if err != nil {
			return err
		}
		affected, err := inventoryDelete.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrInvalidItem
		}
		if _, err := tx.ExecContext(ctx, `
insert into tbl_stallitem(fld_dbid, fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist, fld_price)
values(?, ?, ?, ?, ?, ?, ?, ?)`, dbid, item.ItemID, seqID, item.Count, item.Duration[0], item.Duration[1], blob, price); err != nil {
			return err
		}
		item.SeqID = seqID
		result = protocol.SDStallItem{Item: item, Price: price}
		return nil
	})
	if err != nil {
		return protocol.SDStallItem{}, err
	}
	return result, nil
}

func (r *StallRepository) RemoveItem(ctx context.Context, dbid, itemID, seqID uint32) (protocol.SDItem, bool, error) {
	var item protocol.SDItem
	var blob []byte
	err := r.store.db.QueryRowContext(ctx, `
delete from tbl_stallitem where fld_dbid = ? and fld_itemid = ? and fld_seqid = ?
returning fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist`,
		dbid, itemID, seqID).Scan(&item.ItemID, &item.SeqID, &item.Count, &item.Duration[0], &item.Duration[1], &blob)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.SDItem{}, false, nil
	}
	if err != nil {
		return protocol.SDItem{}, false, fmt.Errorf("remove stall item: %w", err)
	}
	item.ExtAttrList, err = unmarshalExtAttrList(blob)
	if err != nil {
		return protocol.SDItem{}, false, err
	}
	item.SeqID = 0
	return item, true, nil
}

func (r *StallRepository) Start(ctx context.Context, dbid uint32, name string, mapID uint32, x, y int32) (bool, error) {
	if name == "" || len([]byte(name)) >= stallNameMaxBytes || !utf8.ValidString(name) {
		return false, errors.New("invalid stall name")
	}
	result, err := r.store.db.ExecContext(ctx, `
update tbl_stall set fld_name = ?, fld_selling = 1, fld_map = ?, fld_mapx = ?, fld_mapy = ?
where fld_dbid = ? and fld_selling = 0 and exists(select 1 from tbl_stallitem where fld_dbid = ?)`,
		name, mapID, x, y, dbid, dbid)
	if err != nil {
		return false, fmt.Errorf("start stall: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected != 0, nil
}

func (r *StallRepository) Stop(ctx context.Context, dbid uint32) (bool, error) {
	result, err := r.store.db.ExecContext(ctx, `update tbl_stall set fld_selling = 0 where fld_dbid = ? and fld_selling = 1`, dbid)
	if err != nil {
		return false, fmt.Errorf("stop stall: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected != 0, nil
}

// TakeSoldItem removes one stall item for a buyer. Gold transfer and delivery
// are business operations implemented by the calling game service.
func (r *StallRepository) TakeSoldItem(ctx context.Context, sellerDBID, buyerDBID, itemID, seqID uint32) (protocol.SDStallItem, bool, error) {
	if sellerDBID == buyerDBID {
		return protocol.SDStallItem{}, false, errors.New("cannot buy own stall item")
	}
	var result protocol.SDStallItem
	var blob []byte
	err := r.store.db.QueryRowContext(ctx, `
delete from tbl_stallitem
where fld_dbid = ? and fld_itemid = ? and fld_seqid = ?
returning fld_itemid, fld_seqid, fld_count, fld_duration, fld_maxduration, fld_extattrlist, fld_price`,
		sellerDBID, itemID, seqID).Scan(&result.Item.ItemID, &result.Item.SeqID, &result.Item.Count,
		&result.Item.Duration[0], &result.Item.Duration[1], &blob, &result.Price)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.SDStallItem{}, false, nil
	}
	if err != nil {
		return protocol.SDStallItem{}, false, fmt.Errorf("take stall item: %w", err)
	}
	result.Item.ExtAttrList, err = unmarshalExtAttrList(blob)
	if err != nil {
		return protocol.SDStallItem{}, false, err
	}
	result.Item.SeqID = 0
	return result, true, nil
}

func (r *StallRepository) ListSelling(ctx context.Context) ([]protocol.SDStall, error) {
	rows, err := r.store.db.QueryContext(ctx, `select fld_dbid from tbl_stall where fld_selling = 1 order by fld_dbid`)
	if err != nil {
		return nil, fmt.Errorf("list selling stalls: %w", err)
	}
	defer rows.Close()
	var result []protocol.SDStall
	for rows.Next() {
		var dbid uint32
		if err := rows.Scan(&dbid); err != nil {
			return nil, err
		}
		stall, ok, err := r.Load(ctx, dbid)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, stall)
		}
	}
	return result, rows.Err()
}
