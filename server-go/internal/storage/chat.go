package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

const (
	chatDBIDSystem uint32 = 0xFFFFFF01
	chatDBIDGroup  uint32 = 0xFFFFFF02
	chatDBIDAI     uint32 = 0xFFFFFF03

	chatNameSystem = "系统助手"
	chatNameGroup  = "群助手"
	chatNameAI     = "AI助手"
)

// ChatRepository owns chat messages, chat groups and friend/block lists.
type ChatRepository struct {
	store *Store
}

func NewChatRepository(store *Store) *ChatRepository { return &ChatRepository{store: store} }

func (r *ChatRepository) SaveMessage(ctx context.Context, from, to protocol.SDChatPeerID, message string, refer *uint64) (protocol.SDChatMessageDBSeq, error) {
	timestamp := time.Now().UnixNano()
	var referValue any
	if refer != nil {
		referValue = *refer
	}
	var id uint64
	err := r.store.db.QueryRowContext(ctx, `
insert into tbl_chatmessage(fld_timestamp, fld_refer, fld_from, fld_to, fld_message)
values(?, ?, ?, ?, ?) returning fld_id`, timestamp, referValue, from.Data, to.Data, []byte(message)).Scan(&id)
	if err != nil {
		return protocol.SDChatMessageDBSeq{}, fmt.Errorf("save chat message: %w", err)
	}
	return protocol.SDChatMessageDBSeq{ID: id, Timestamp: uint64(timestamp)}, nil
}

func (r *ChatRepository) QueryMessage(ctx context.Context, id uint64) (protocol.SDChatMessage, bool, error) {
	row := r.store.db.QueryRowContext(ctx, `
select fld_id, fld_timestamp, fld_refer, fld_from, fld_to, fld_message
from tbl_chatmessage where fld_id = ?`, id)
	return scanChatMessage(row)
}

type rowScanner interface{ Scan(...any) error }

func scanChatMessage(scanner rowScanner) (protocol.SDChatMessage, bool, error) {
	var message protocol.SDChatMessage
	var id, timestamp uint64
	var refer sql.NullInt64
	var from, to uint64
	var body []byte
	if err := scanner.Scan(&id, &timestamp, &refer, &from, &to, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return protocol.SDChatMessage{}, false, nil
		}
		return protocol.SDChatMessage{}, false, err
	}
	message.HasSeq = true
	message.Seq = protocol.SDChatMessageDBSeq{ID: id, Timestamp: timestamp}
	if refer.Valid {
		message.HasRefer = true
		message.Refer = uint64(refer.Int64)
	}
	message.From = protocol.SDChatPeerID{Data: from}
	message.To = protocol.SDChatPeerID{Data: to}
	message.Message = string(body)
	return message, true, nil
}

func specialChatPeer(id uint32) (protocol.SDChatPeer, bool) {
	switch id {
	case chatDBIDSystem:
		return protocol.SDChatPeer{ID: id, Name: chatNameSystem, DespKind: protocol.SDChatPeerDespNone}, true
	case chatDBIDGroup:
		return protocol.SDChatPeer{ID: id, Name: chatNameGroup, DespKind: protocol.SDChatPeerDespNone}, true
	case chatDBIDAI:
		return protocol.SDChatPeer{ID: id, Name: chatNameAI, DespKind: protocol.SDChatPeerDespNone}, true
	default:
		return protocol.SDChatPeer{}, false
	}
}

func (r *ChatRepository) LoadPeer(ctx context.Context, cpid protocol.SDChatPeerID) (protocol.SDChatPeer, bool, error) {
	switch cpid.Type() {
	case protocol.CPR_GROUP:
		var peer protocol.SDChatPeer
		var creator uint32
		var createtime uint64
		err := r.store.db.QueryRowContext(ctx, `select fld_id, fld_name, fld_creator, fld_createtime from tbl_chatgroup where fld_id = ?`, cpid.ID()).
			Scan(&peer.ID, &peer.Name, &creator, &createtime)
		if errors.Is(err, sql.ErrNoRows) {
			return protocol.SDChatPeer{}, false, nil
		}
		if err != nil {
			return protocol.SDChatPeer{}, false, fmt.Errorf("load chat group peer: %w", err)
		}
		peer.DespKind = protocol.SDChatPeerDespGroup
		peer.GroupVar = protocol.SDChatPeerGroupVar{Creator: creator, Createtime: createtime}
		return peer, true, nil

	case protocol.CPR_PLAYER:
		var peer protocol.SDChatPeer
		var gender uint8
		var job int32
		err := r.store.db.QueryRowContext(ctx, `select fld_dbid, fld_name, fld_gender, fld_job from tbl_char where fld_dbid = ?`, cpid.ID()).
			Scan(&peer.ID, &peer.Name, &gender, &job)
		if errors.Is(err, sql.ErrNoRows) {
			return protocol.SDChatPeer{}, false, nil
		}
		if err != nil {
			return protocol.SDChatPeer{}, false, fmt.Errorf("load player peer: %w", err)
		}
		peer.DespKind = protocol.SDChatPeerDespPlayer
		peer.PlayerVar = protocol.SDChatPeerPlayerVar{Gender: gender != 0, Job: job}
		return peer, true, nil

	case protocol.CPR_SPECIAL:
		peer, ok := specialChatPeer(cpid.ID())
		return peer, ok, nil
	default:
		return protocol.SDChatPeer{}, false, nil
	}
}

func (r *ChatRepository) LoadGroupMemberList(ctx context.Context, groupID uint32) ([]uint32, error) {
	rows, err := r.store.db.QueryContext(ctx, `select fld_member from tbl_chatgroupmember where fld_group = ? order by fld_jointime, fld_member`, groupID)
	if err != nil {
		return nil, fmt.Errorf("load group members: %w", err)
	}
	defer rows.Close()
	var result []uint32
	for rows.Next() {
		var dbid uint32
		if err := rows.Scan(&dbid); err != nil {
			return nil, err
		}
		result = append(result, dbid)
	}
	return result, rows.Err()
}

func (r *ChatRepository) IsGroupMember(ctx context.Context, playerDBID, groupID uint32) (bool, error) {
	var count int
	if err := r.store.db.QueryRowContext(ctx, `select count(*) from tbl_chatgroupmember where fld_group = ? and fld_member = ?`, groupID, playerDBID).Scan(&count); err != nil {
		return false, fmt.Errorf("check group member: %w", err)
	}
	return count != 0, nil
}

func (r *ChatRepository) LoadGroupList(ctx context.Context, memberDBID uint32) (protocol.SDChatPeerList, error) {
	rows, err := r.store.db.QueryContext(ctx, `
select fld_id, fld_name, fld_creator, fld_createtime
from tbl_chatgroup
where fld_id in (select fld_group from tbl_chatgroupmember where fld_member = ?)
order by fld_id`, memberDBID)
	if err != nil {
		return nil, fmt.Errorf("load chat groups: %w", err)
	}
	defer rows.Close()
	var result protocol.SDChatPeerList
	for rows.Next() {
		var peer protocol.SDChatPeer
		var creator uint32
		var createtime uint64
		if err := rows.Scan(&peer.ID, &peer.Name, &creator, &createtime); err != nil {
			return nil, err
		}
		peer.DespKind = protocol.SDChatPeerDespGroup
		peer.GroupVar = protocol.SDChatPeerGroupVar{Creator: creator, Createtime: createtime}
		result = append(result, peer)
	}
	return result, rows.Err()
}

func (r *ChatRepository) QueryPeerList(ctx context.Context, query string, includePlayer, includeGroup bool) (protocol.SDChatPeerList, error) {
	if strings.TrimSpace(query) == "" || (!includePlayer && !includeGroup) {
		return nil, nil
	}
	onlyDigits := true
	for _, ch := range query {
		if ch < '0' || ch > '9' {
			onlyDigits = false
			break
		}
	}
	var result protocol.SDChatPeerList
	if includePlayer {
		sqlQuery := `select fld_dbid, fld_name, fld_gender, fld_job from tbl_char where instr(fld_name, ?) > 0`
		args := []any{query}
		if onlyDigits {
			if id, err := strconv.ParseUint(query, 10, 32); err == nil {
				sqlQuery = `select fld_dbid, fld_name, fld_gender, fld_job from tbl_char where fld_dbid = ? or instr(fld_name, ?) > 0`
				args = []any{id, query}
			}
		}
		rows, err := r.store.db.QueryContext(ctx, sqlQuery, args...)
		if err != nil {
			return nil, fmt.Errorf("query player peers: %w", err)
		}
		for rows.Next() {
			var peer protocol.SDChatPeer
			var gender uint8
			var job int32
			if err := rows.Scan(&peer.ID, &peer.Name, &gender, &job); err != nil {
				rows.Close()
				return nil, err
			}
			peer.DespKind = protocol.SDChatPeerDespPlayer
			peer.PlayerVar = protocol.SDChatPeerPlayerVar{Gender: gender != 0, Job: job}
			result = append(result, peer)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	if includeGroup {
		sqlQuery := `select fld_id, fld_name, fld_creator, fld_createtime from tbl_chatgroup where instr(fld_name, ?) > 0`
		args := []any{query}
		if onlyDigits {
			if id, err := strconv.ParseUint(query, 10, 32); err == nil {
				sqlQuery = `select fld_id, fld_name, fld_creator, fld_createtime from tbl_chatgroup where fld_id = ? or instr(fld_name, ?) > 0`
				args = []any{id, query}
			}
		}
		rows, err := r.store.db.QueryContext(ctx, sqlQuery, args...)
		if err != nil {
			return nil, fmt.Errorf("query group peers: %w", err)
		}
		for rows.Next() {
			var peer protocol.SDChatPeer
			var creator uint32
			var createtime uint64
			if err := rows.Scan(&peer.ID, &peer.Name, &creator, &createtime); err != nil {
				rows.Close()
				return nil, err
			}
			peer.DespKind = protocol.SDChatPeerDespGroup
			peer.GroupVar = protocol.SDChatPeerGroupVar{Creator: creator, Createtime: createtime}
			result = append(result, peer)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *ChatRepository) RetrieveLatest(ctx context.Context, playerDBID uint32, cpids []uint64, limit int, includeSend, includeRecv bool) (protocol.SDChatMessageList, error) {
	if len(cpids) == 0 || (!includeSend && !includeRecv) {
		return nil, nil
	}
	playerCPID := protocol.NewSDChatPeerID(protocol.CPR_PLAYER, playerDBID)
	var result protocol.SDChatMessageList
	for _, otherData := range cpids {
		other := protocol.SDChatPeerID{Data: otherData}
		if other.Type() == protocol.CPR_GROUP {
			ok, err := r.IsGroupMember(ctx, playerDBID, other.ID())
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		where := make([]string, 0, 2)
		args := make([]any, 0, 4)
		if includeSend {
			where = append(where, `(fld_from = ? and fld_to = ?)`)
			args = append(args, playerCPID.Data, other.Data)
		}
		if includeRecv {
			if other.Type() == protocol.CPR_GROUP {
				where = append(where, `(fld_to = ?)`)
				args = append(args, other.Data)
			} else {
				where = append(where, `(fld_from = ? and fld_to = ?)`)
				args = append(args, other.Data, playerCPID.Data)
			}
		}
		query := `select fld_id, fld_timestamp, fld_refer, fld_from, fld_to, fld_message from tbl_chatmessage where ` + strings.Join(where, " or ") + ` order by fld_timestamp desc`
		if limit > 0 {
			query += ` limit ?`
			args = append(args, limit)
		}
		rows, err := r.store.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("retrieve latest chat messages: %w", err)
		}
		for rows.Next() {
			message, _, err := scanChatMessage(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			result = append(result, message)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *ChatRepository) CreateGroup(ctx context.Context, creatorDBID uint32, name string, memberIDs []uint32) (protocol.SDChatPeer, error) {
	if strings.TrimSpace(name) == "" {
		return protocol.SDChatPeer{}, errors.New("chat group name is empty")
	}
	timestamp := time.Now().UnixNano()
	var groupID uint32
	err := r.store.InTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `insert into tbl_chatgroup(fld_creator, fld_createtime, fld_name) values(?, ?, ?) returning fld_id`, creatorDBID, timestamp, name).Scan(&groupID); err != nil {
			return err
		}
		members := append([]uint32(nil), memberIDs...)
		foundCreator := false
		for _, dbid := range members {
			if dbid == creatorDBID {
				foundCreator = true
				break
			}
		}
		if !foundCreator {
			members = append(members, creatorDBID)
		}
		for _, dbid := range members {
			if _, err := tx.ExecContext(ctx, `insert or ignore into tbl_chatgroupmember(fld_group, fld_member, fld_permission, fld_jointime) values(?, ?, 0, ?)`, groupID, dbid, timestamp); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return protocol.SDChatPeer{}, fmt.Errorf("create chat group: %w", err)
	}
	peer := protocol.SDChatPeer{ID: groupID, Name: name, DespKind: protocol.SDChatPeerDespGroup}
	peer.GroupVar = protocol.SDChatPeerGroupVar{Creator: creatorDBID, Createtime: uint64(timestamp)}
	return peer, nil
}

func (r *ChatRepository) AddFriend(ctx context.Context, dbid, friendDBID uint32) error {
	if dbid == friendDBID {
		return errors.New("cannot add self as friend")
	}
	_, err := r.store.db.ExecContext(ctx, `insert or ignore into tbl_friend(fld_dbid, fld_friend) values(?, ?)`, dbid, friendDBID)
	if err != nil {
		return fmt.Errorf("add friend: %w", err)
	}
	return nil
}

func (r *ChatRepository) RemoveFriend(ctx context.Context, dbid, friendDBID uint32) (bool, error) {
	result, err := r.store.db.ExecContext(ctx, `delete from tbl_friend where fld_dbid = ? and fld_friend = ?`, dbid, friendDBID)
	if err != nil {
		return false, fmt.Errorf("remove friend: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected != 0, nil
}

func (r *ChatRepository) ListFriends(ctx context.Context, dbid uint32) (protocol.SDChatPeerList, error) {
	rows, err := r.store.db.QueryContext(ctx, `
select fld_dbid, fld_name, fld_gender, fld_job from tbl_char
where fld_dbid in (select fld_friend from tbl_friend where fld_dbid = ?)
order by fld_dbid`, dbid)
	if err != nil {
		return nil, fmt.Errorf("list friends: %w", err)
	}
	var result protocol.SDChatPeerList
	for rows.Next() {
		var peer protocol.SDChatPeer
		var gender uint8
		var job int32
		if err := rows.Scan(&peer.ID, &peer.Name, &gender, &job); err != nil {
			rows.Close()
			return nil, err
		}
		peer.DespKind = protocol.SDChatPeerDespPlayer
		peer.PlayerVar = protocol.SDChatPeerPlayerVar{Gender: gender != 0, Job: job}
		result = append(result, peer)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	groups, err := r.LoadGroupList(ctx, dbid)
	if err != nil {
		return nil, err
	}
	return append(result, groups...), nil
}

func (r *ChatRepository) BlockPlayer(ctx context.Context, dbid, blockedDBID uint32) error {
	if dbid == blockedDBID {
		return errors.New("cannot block self")
	}
	_, err := r.store.db.ExecContext(ctx, `insert or ignore into tbl_blacklist(fld_dbid, fld_blocked) values(?, ?)`, dbid, blockedDBID)
	if err != nil {
		return fmt.Errorf("block player: %w", err)
	}
	return nil
}

func (r *ChatRepository) UnblockPlayer(ctx context.Context, dbid, blockedDBID uint32) (bool, error) {
	result, err := r.store.db.ExecContext(ctx, `delete from tbl_blacklist where fld_dbid = ? and fld_blocked = ?`, dbid, blockedDBID)
	if err != nil {
		return false, fmt.Errorf("unblock player: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected != 0, nil
}

func (r *ChatRepository) ListBlocked(ctx context.Context, dbid uint32) ([]uint32, error) {
	rows, err := r.store.db.QueryContext(ctx, `select fld_blocked from tbl_blacklist where fld_dbid = ? order by fld_blocked`, dbid)
	if err != nil {
		return nil, fmt.Errorf("list blocked players: %w", err)
	}
	defer rows.Close()
	var result []uint32
	for rows.Next() {
		var blocked uint32
		if err := rows.Scan(&blocked); err != nil {
			return nil, err
		}
		result = append(result, blocked)
	}
	return result, rows.Err()
}
