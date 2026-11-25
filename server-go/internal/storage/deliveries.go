package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

var (
	ErrDeliveryNotFound = errors.New("delivery not found")
	ErrDeliveryClaimed  = errors.New("delivery already claimed")
	ErrDeliveryEmpty    = errors.New("delivery item list is empty")
)

// Delivery is one persisted offline delivery.
type Delivery struct {
	Record        string
	RecipientDBID uint32
	MessageID     *uint64
	Timestamp     uint64
	Claimed       bool
	ClaimTime     *uint64
	Payload       []byte
}

// DeliveryRepository owns tbl_delivery access.
type DeliveryRepository struct {
	store *Store
}

func NewDeliveryRepository(store *Store) *DeliveryRepository {
	return &DeliveryRepository{store: store}
}

func randomDeliveryRecord() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func encodeDeliveryItems(items []protocol.SDItem) ([]byte, error) {
	if len(items) == 0 {
		return nil, ErrDeliveryEmpty
	}
	for _, item := range items {
		if err := validateItem(item, false); err != nil {
			return nil, err
		}
	}
	return MarshalCerealBlob(protocol.SDShowSecuredItemList{ItemList: items})
}

func decodeDeliveryItems(blob []byte) ([]protocol.SDItem, error) {
	var value protocol.SDShowSecuredItemList
	if err := UnmarshalCerealBlob(blob, &value); err != nil {
		return nil, fmt.Errorf("decode delivery payload: %w", err)
	}
	return value.ItemList, nil
}

func (r *DeliveryRepository) Create(ctx context.Context, recipientDBID uint32, items []protocol.SDItem, messageID *uint64) (Delivery, error) {
	payload, err := encodeDeliveryItems(items)
	if err != nil {
		return Delivery{}, err
	}
	timestamp := uint64(time.Now().UnixNano())
	for attempt := 0; attempt < 16; attempt++ {
		record, err := randomDeliveryRecord()
		if err != nil {
			return Delivery{}, fmt.Errorf("generate delivery record: %w", err)
		}
		var messageValue any
		if messageID != nil {
			messageValue = *messageID
		}
		var createdAt int64
		var returned string
		err = r.store.db.QueryRowContext(ctx, `
insert into tbl_delivery(fld_record, fld_dbid, fld_messageid, fld_timestamp, fld_payload)
values(?, ?, ?, ?, ?)
on conflict(fld_record) do nothing
returning fld_record, fld_timestamp`, record, recipientDBID, messageValue, timestamp, payload).Scan(&returned, &createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return Delivery{}, fmt.Errorf("create delivery: %w", err)
		}
		return Delivery{
			Record: returned, RecipientDBID: recipientDBID, MessageID: messageID,
			Timestamp: uint64(createdAt), Payload: payload,
		}, nil
	}
	return Delivery{}, errors.New("failed to allocate delivery record")
}

func scanDelivery(scanner rowScanner) (Delivery, error) {
	var delivery Delivery
	var messageID, claimTime sql.NullInt64
	var claimed int
	err := scanner.Scan(&delivery.Record, &delivery.RecipientDBID, &messageID, &delivery.Timestamp, &claimed, &claimTime, &delivery.Payload)
	if err != nil {
		return Delivery{}, err
	}
	if messageID.Valid {
		value := uint64(messageID.Int64)
		delivery.MessageID = &value
	}
	if claimTime.Valid {
		value := uint64(claimTime.Int64)
		delivery.ClaimTime = &value
	}
	delivery.Claimed = claimed != 0
	return delivery, nil
}

func (r *DeliveryRepository) Load(ctx context.Context, dbid uint32, record string) (Delivery, error) {
	row := r.store.db.QueryRowContext(ctx, `
select fld_record, fld_dbid, fld_messageid, fld_timestamp, fld_claimed, fld_claimtime, fld_payload
from tbl_delivery where fld_record = ? and fld_dbid = ?`, record, dbid)
	delivery, err := scanDelivery(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Delivery{}, ErrDeliveryNotFound
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("load delivery: %w", err)
	}
	return delivery, nil
}

func (r *DeliveryRepository) ListUnclaimed(ctx context.Context, dbid uint32) ([]Delivery, error) {
	rows, err := r.store.db.QueryContext(ctx, `
select fld_record, fld_dbid, fld_messageid, fld_timestamp, fld_claimed, fld_claimtime, fld_payload
from tbl_delivery where fld_dbid = ? and fld_claimed = 0 order by fld_timestamp, fld_record`, dbid)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()
	var result []Delivery
	for rows.Next() {
		delivery, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, delivery)
	}
	return result, rows.Err()
}

// BindMessage 把投递记录关联到一条聊天消息（deliverydb.cpp:72 的 update）。
//
// ★ 这一步不是可选的：
//
//	客户端点「收取」时带的 record 来自**聊天消息正文里的 XML**。
//	如果 fld_messageid 没回填，消息和投递记录就没有关联，
//	以后想「删消息同时删投递」或者「按消息查投递」都做不到。
//	当前实现里它只影响一致性检查，但仍要写 —— 原版写了。
func (r *DeliveryRepository) BindMessage(ctx context.Context, record string, messageID uint64) error {
	result, err := r.store.db.ExecContext(ctx,
		`update tbl_delivery set fld_messageid = ? where fld_record = ?`, messageID, record)
	if err != nil {
		return fmt.Errorf("bind delivery message: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrDeliveryNotFound
	}
	return nil
}

// Unclaim 把一条已领取的投递改回未领取（对应原版的事务回滚）。
//
// ★ 为什么需要「领取后再回滚」：
//
//	原版 claimDelivery 在一个事务里做「取投递 → 检查金币不溢出 → 提交」，
//	金币溢出时事务回滚，投递保持未领取。
//	Go 侧 storage 是单语句 autocommit，所以只能「先领、失败再退」。
//
//	只有「金币到账会超过 SYS_MAXGOLD」这一种情况需要回滚，
//	且概率极低（要 9000 万亿金币），但漏了就是物品凭空消失。
func (r *DeliveryRepository) Unclaim(ctx context.Context, record string) error {
	_, err := r.store.db.ExecContext(ctx, `
update tbl_delivery set fld_claimed = 0, fld_claimtime = null
where fld_record = ?`, record)
	if err != nil {
		return fmt.Errorf("unclaim delivery: %w", err)
	}
	return nil
}

func (r *DeliveryRepository) Claim(ctx context.Context, dbid uint32, record string) ([]protocol.SDItem, error) {
	claimTime := time.Now().UnixNano()
	var payload []byte
	err := r.store.db.QueryRowContext(ctx, `
update tbl_delivery set fld_claimed = 1, fld_claimtime = ?
where fld_record = ? and fld_dbid = ? and fld_claimed = 0
returning fld_payload`, claimTime, record, dbid).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		delivery, loadErr := r.Load(ctx, dbid, record)
		if errors.Is(loadErr, ErrDeliveryNotFound) {
			return nil, ErrDeliveryNotFound
		}
		if loadErr != nil {
			return nil, loadErr
		}
		if delivery.Claimed {
			return nil, ErrDeliveryClaimed
		}
		return nil, ErrDeliveryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("claim delivery: %w", err)
	}
	return decodeDeliveryItems(payload)
}
