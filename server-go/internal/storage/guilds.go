package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

const (
	guildRankLeader int32 = 1
	guildRankMember int32 = 2

	guildNameMaxBytes   = 32
	guildTitleMaxBytes  = 32
	guildNoticeMaxBytes = 1024
	guildWarDuration    = int64(60 * 60)
)

// GuildErrorCode mirrors the legacy GUILDERR_* wire enum.
type GuildErrorCode uint8

const (
	GuildErrNone GuildErrorCode = iota
	GuildErrInGuild
	GuildErrNoGuild
	GuildErrNotLeader
	GuildErrBadName
	GuildErrNameTaken
	GuildErrNoGold
	GuildErrNoItem
	GuildErrBadTarget
	GuildErrTargetInGuild
	GuildErrNotMember
	GuildErrLeaderLeave
	GuildErrNoInvite
	GuildErrRejected
	GuildErrNoPlayer
	GuildErrTargetNotAllowed
	GuildErrBadTitle
	GuildErrBadNotice
	GuildErrSelfWar
)

type GuildMembership struct {
	GuildID   uint32
	GuildName string
	Rank      int32
	Title     string
}

type GuildInvite struct {
	DBID      uint32
	GuildID   uint32
	InviterID uint32
	InviteAt  int64
}

// GuildRepository owns guild membership, invite and war persistence.
type GuildRepository struct {
	store *Store
}

func NewGuildRepository(store *Store) *GuildRepository { return &GuildRepository{store: store} }

type guildRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadGuildIDQuery(ctx context.Context, queryer guildRowQueryer, dbid uint32) (uint32, error) {
	var guildID uint32
	err := queryer.QueryRowContext(ctx, `select fld_guild from tbl_guildmember where fld_dbid = ?`, dbid).Scan(&guildID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("load guild id: %w", err)
	}
	return guildID, nil
}

func loadLedGuildIDQuery(ctx context.Context, queryer guildRowQueryer, dbid uint32) (uint32, GuildErrorCode, error) {
	var guildID uint32
	var rank int32
	err := queryer.QueryRowContext(ctx, `select fld_guild, fld_rank from tbl_guildmember where fld_dbid = ?`, dbid).Scan(&guildID, &rank)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, GuildErrNoGuild, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("load guild leadership: %w", err)
	}
	if rank != guildRankLeader {
		return 0, GuildErrNotLeader, nil
	}
	return guildID, GuildErrNone, nil
}

func guildLabelValid(value string, limit int) bool {
	if value == "" || len([]byte(value)) >= limit || !utf8.ValidString(value) {
		return false
	}
	for _, ch := range value {
		if ch <= 0x20 || ch == 0x7f || strings.ContainsRune(`<>&"'`, ch) {
			return false
		}
	}
	return true
}

func GuildNameValid(name string) bool   { return guildLabelValid(name, guildNameMaxBytes) }
func GuildTitleValid(title string) bool { return guildLabelValid(title, guildTitleMaxBytes) }

func GuildNoticeValid(notice string) bool {
	if len([]byte(notice)) >= guildNoticeMaxBytes || !utf8.ValidString(notice) {
		return false
	}
	for _, ch := range notice {
		if (ch < 0x20 && ch != '\n') || ch == 0x7f {
			return false
		}
	}
	return true
}

func defaultGuildTitle(rank int32) string {
	switch rank {
	case guildRankLeader:
		return "掌门人"
	case guildRankMember:
		return "成员"
	default:
		return ""
	}
}

func (r *GuildRepository) loadLedGuildID(ctx context.Context, dbid uint32) (uint32, GuildErrorCode, error) {
	return loadLedGuildIDQuery(ctx, r.store.db, dbid)
}

func (r *GuildRepository) LoadMembership(ctx context.Context, dbid uint32) (GuildMembership, bool, error) {
	var membership GuildMembership
	err := r.store.db.QueryRowContext(ctx, `
select g.fld_id, g.fld_name, gm.fld_rank, gm.fld_title
from tbl_guildmember gm join tbl_guild g on g.fld_id = gm.fld_guild
where gm.fld_dbid = ?`, dbid).Scan(&membership.GuildID, &membership.GuildName, &membership.Rank, &membership.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return GuildMembership{}, false, nil
	}
	if err != nil {
		return GuildMembership{}, false, fmt.Errorf("load guild membership: %w", err)
	}
	return membership, true, nil
}

func (r *GuildRepository) LoadGuildID(ctx context.Context, dbid uint32) (uint32, error) {
	return loadGuildIDQuery(ctx, r.store.db, dbid)
}

func (r *GuildRepository) LoadInfo(ctx context.Context, guildID uint32) (protocol.SDGuildInfo, error) {
	var info protocol.SDGuildInfo
	err := r.store.db.QueryRowContext(ctx, `select fld_name, fld_notice from tbl_guild where fld_id = ?`, guildID).Scan(&info.Name, &info.Notice)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.SDGuildInfo{}, nil
	}
	if err != nil {
		return protocol.SDGuildInfo{}, fmt.Errorf("load guild info: %w", err)
	}
	info.GuildID = guildID
	rows, err := r.store.db.QueryContext(ctx, `
select gm.fld_dbid, c.fld_name, gm.fld_rank, gm.fld_title
from tbl_guildmember gm join tbl_char c on c.fld_dbid = gm.fld_dbid
where gm.fld_guild = ?
order by gm.fld_rank, gm.fld_jointime, gm.fld_dbid`, guildID)
	if err != nil {
		return protocol.SDGuildInfo{}, fmt.Errorf("load guild members: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var member protocol.SDGuildMember
		if err := rows.Scan(&member.DBID, &member.Name, &member.Rank, &member.Title); err != nil {
			return protocol.SDGuildInfo{}, fmt.Errorf("scan guild member: %w", err)
		}
		info.MemberList = append(info.MemberList, member)
	}
	if err := rows.Err(); err != nil {
		return protocol.SDGuildInfo{}, fmt.Errorf("iterate guild members: %w", err)
	}
	return info, nil
}

func (r *GuildRepository) LoadMemberList(ctx context.Context, guildID uint32) ([]uint32, error) {
	rows, err := r.store.db.QueryContext(ctx, `select fld_dbid from tbl_guildmember where fld_guild = ? order by fld_jointime, fld_dbid`, guildID)
	if err != nil {
		return nil, fmt.Errorf("load guild member list: %w", err)
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

func (r *GuildRepository) FindCharacterDBID(ctx context.Context, name string) (uint32, error) {
	var dbid uint32
	err := r.store.db.QueryRowContext(ctx, `select fld_dbid from tbl_char where fld_name = ?`, name).Scan(&dbid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find character: %w", err)
	}
	return dbid, nil
}

func insertGuildMember(ctx context.Context, tx *sql.Tx, guildID, dbid uint32, rank int32, now int64) error {
	_, err := tx.ExecContext(ctx, `insert into tbl_guildmember(fld_dbid, fld_guild, fld_rank, fld_title, fld_jointime) values(?, ?, ?, ?, ?)`,
		dbid, guildID, rank, defaultGuildTitle(rank), now)
	return err
}

func (r *GuildRepository) Create(ctx context.Context, leaderDBID uint32, name string) (uint32, GuildErrorCode, error) {
	if !GuildNameValid(name) {
		return 0, GuildErrBadName, nil
	}
	var guildID uint32
	err := r.store.InTx(ctx, func(tx *sql.Tx) error {
		if guild, err := loadGuildIDQuery(ctx, tx, leaderDBID); err != nil {
			return err
		} else if guild != 0 {
			return errGuildCode(GuildErrInGuild)
		}
		var exists int
		if err := tx.QueryRowContext(ctx, `select count(*) from tbl_guild where fld_name = ?`, name).Scan(&exists); err != nil {
			return err
		}
		if exists != 0 {
			return errGuildCode(GuildErrNameTaken)
		}
		if err := tx.QueryRowContext(ctx, `insert into tbl_guild(fld_name, fld_createtime) values(?, ?) returning fld_id`, name, time.Now().Unix()).Scan(&guildID); err != nil {
			return err
		}
		if err := insertGuildMember(ctx, tx, guildID, leaderDBID, guildRankLeader, time.Now().Unix()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `delete from tbl_guildinvite where fld_dbid = ?`, leaderDBID)
		return err
	})
	return guildID, codeFromError(err), normalizeGuildError(err)
}

type guildCodeError struct{ code GuildErrorCode }

func (e guildCodeError) Error() string       { return fmt.Sprintf("guild error %d", e.code) }
func errGuildCode(code GuildErrorCode) error { return guildCodeError{code: code} }
func codeFromError(err error) GuildErrorCode {
	var codeErr guildCodeError
	if errors.As(err, &codeErr) {
		return codeErr.code
	}
	return 0
}
func normalizeGuildError(err error) error {
	var codeErr guildCodeError
	if errors.As(err, &codeErr) {
		return nil
	}
	return err
}

func (r *GuildRepository) Invite(ctx context.Context, leaderDBID, targetDBID uint32) (GuildErrorCode, error) {
	if targetDBID == 0 || targetDBID == leaderDBID {
		return GuildErrBadTarget, nil
	}
	err := r.store.InTx(ctx, func(tx *sql.Tx) error {
		guildID, code, err := loadLedGuildIDQuery(ctx, tx, leaderDBID)
		if err != nil || code != GuildErrNone {
			return errGuildCode(code)
		}
		targetGuild, err := loadGuildIDQuery(ctx, tx, targetDBID)
		if err != nil {
			return err
		}
		if targetGuild != 0 {
			return errGuildCode(GuildErrTargetInGuild)
		}
		_, err = tx.ExecContext(ctx, `insert or replace into tbl_guildinvite(fld_dbid, fld_guild, fld_inviter, fld_invitetime) values(?, ?, ?, ?)`,
			targetDBID, guildID, leaderDBID, time.Now().Unix())
		return err
	})
	return codeFromError(err), normalizeGuildError(err)
}

func (r *GuildRepository) AcceptInvite(ctx context.Context, dbid, guildID uint32) (GuildErrorCode, error) {
	err := r.store.InTx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `select count(*) from tbl_guildinvite where fld_dbid = ? and fld_guild = ?`, dbid, guildID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return errGuildCode(GuildErrNoInvite)
		}
		current, err := loadGuildIDQuery(ctx, tx, dbid)
		if err != nil {
			return err
		}
		if current != 0 {
			return errGuildCode(GuildErrInGuild)
		}
		if err := insertGuildMember(ctx, tx, guildID, dbid, guildRankMember, time.Now().Unix()); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `delete from tbl_guildinvite where fld_dbid = ?`, dbid)
		return err
	})
	return codeFromError(err), normalizeGuildError(err)
}

func (r *GuildRepository) RejectInvite(ctx context.Context, dbid, guildID uint32) (uint32, bool, error) {
	var inviter uint32
	err := r.store.db.QueryRowContext(ctx, `delete from tbl_guildinvite where fld_dbid = ? and fld_guild = ? returning fld_inviter`, dbid, guildID).Scan(&inviter)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("reject guild invite: %w", err)
	}
	return inviter, true, nil
}

func (r *GuildRepository) Leave(ctx context.Context, dbid uint32) (GuildErrorCode, error) {
	membership, ok, err := r.LoadMembership(ctx, dbid)
	if err != nil {
		return 0, err
	}
	if !ok {
		return GuildErrNoGuild, nil
	}
	if membership.Rank == guildRankLeader {
		return GuildErrLeaderLeave, nil
	}
	_, err = r.store.db.ExecContext(ctx, `delete from tbl_guildmember where fld_dbid = ?`, dbid)
	if err != nil {
		return 0, fmt.Errorf("leave guild: %w", err)
	}
	return GuildErrNone, nil
}

func (r *GuildRepository) Kick(ctx context.Context, leaderDBID, targetDBID uint32) (GuildErrorCode, error) {
	if targetDBID == 0 || targetDBID == leaderDBID {
		return GuildErrBadTarget, nil
	}
	guildID, code, err := r.loadLedGuildID(ctx, leaderDBID)
	if err != nil || code != GuildErrNone {
		return code, err
	}
	result, err := r.store.db.ExecContext(ctx, `delete from tbl_guildmember where fld_dbid = ? and fld_guild = ?`, targetDBID, guildID)
	if err != nil {
		return 0, fmt.Errorf("kick guild member: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return GuildErrNotMember, nil
	}
	return GuildErrNone, nil
}

func setGuildRank(ctx context.Context, tx *sql.Tx, dbid uint32, rank int32) error {
	_, err := tx.ExecContext(ctx, `update tbl_guildmember set fld_rank = ?, fld_title = ? where fld_dbid = ?`, rank, defaultGuildTitle(rank), dbid)
	return err
}

func (r *GuildRepository) TransferLeader(ctx context.Context, leaderDBID, targetDBID uint32) (GuildErrorCode, error) {
	if targetDBID == 0 || targetDBID == leaderDBID {
		return GuildErrBadTarget, nil
	}
	err := r.store.InTx(ctx, func(tx *sql.Tx) error {
		guildID, code, err := loadLedGuildIDQuery(ctx, tx, leaderDBID)
		if err != nil || code != GuildErrNone {
			return errGuildCode(code)
		}
		targetGuild, err := loadGuildIDQuery(ctx, tx, targetDBID)
		if err != nil {
			return err
		}
		if targetGuild != guildID {
			return errGuildCode(GuildErrNotMember)
		}
		if err := setGuildRank(ctx, tx, leaderDBID, guildRankMember); err != nil {
			return err
		}
		return setGuildRank(ctx, tx, targetDBID, guildRankLeader)
	})
	return codeFromError(err), normalizeGuildError(err)
}

func (r *GuildRepository) SetNotice(ctx context.Context, leaderDBID uint32, notice string) (GuildErrorCode, error) {
	if !GuildNoticeValid(notice) {
		return GuildErrBadNotice, nil
	}
	guildID, code, err := r.loadLedGuildID(ctx, leaderDBID)
	if err != nil || code != GuildErrNone {
		return code, err
	}
	if _, err := r.store.db.ExecContext(ctx, `update tbl_guild set fld_notice = ? where fld_id = ?`, notice, guildID); err != nil {
		return 0, fmt.Errorf("set guild notice: %w", err)
	}
	return GuildErrNone, nil
}

func (r *GuildRepository) SetMemberTitle(ctx context.Context, leaderDBID, targetDBID uint32, title string) (GuildErrorCode, error) {
	if targetDBID == 0 {
		return GuildErrBadTarget, nil
	}
	if !GuildTitleValid(title) {
		return GuildErrBadTitle, nil
	}
	guildID, code, err := r.loadLedGuildID(ctx, leaderDBID)
	if err != nil || code != GuildErrNone {
		return code, err
	}
	result, err := r.store.db.ExecContext(ctx, `update tbl_guildmember set fld_title = ? where fld_dbid = ? and fld_guild = ?`, title, targetDBID, guildID)
	if err != nil {
		return 0, fmt.Errorf("set guild member title: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return GuildErrNotMember, nil
	}
	return GuildErrNone, nil
}

func (r *GuildRepository) Disband(ctx context.Context, leaderDBID uint32) ([]uint32, GuildErrorCode, error) {
	guildID, code, err := r.loadLedGuildID(ctx, leaderDBID)
	if err != nil || code != GuildErrNone {
		return nil, code, err
	}
	members, err := r.LoadMemberList(ctx, guildID)
	if err != nil {
		return nil, 0, err
	}
	var others []uint32
	for _, member := range members {
		if member != leaderDBID {
			others = append(others, member)
		}
	}
	if _, err := r.store.db.ExecContext(ctx, `delete from tbl_guild where fld_id = ?`, guildID); err != nil {
		return nil, 0, fmt.Errorf("disband guild: %w", err)
	}
	return others, GuildErrNone, nil
}

func (r *GuildRepository) DeclareWar(ctx context.Context, leaderDBID uint32, enemyName string, now int64) (uint32, GuildErrorCode, error) {
	if now == 0 {
		now = time.Now().Unix()
	}
	guildID, code, err := r.loadLedGuildID(ctx, leaderDBID)
	if err != nil || code != GuildErrNone {
		return 0, code, err
	}
	enemyID, err := r.FindGuildIDByName(ctx, enemyName)
	if err != nil {
		return 0, 0, err
	}
	if enemyID == 0 {
		return 0, GuildErrBadTarget, nil
	}
	if enemyID == guildID {
		return 0, GuildErrSelfWar, nil
	}
	first, second := guildID, enemyID
	if first > second {
		first, second = second, first
	}
	_, err = r.store.db.ExecContext(ctx, `insert or replace into tbl_guildwar(fld_guild1, fld_guild2, fld_endtime) values(?, ?, ?)`, first, second, now+guildWarDuration)
	if err != nil {
		return 0, 0, fmt.Errorf("declare guild war: %w", err)
	}
	return enemyID, GuildErrNone, nil
}

func (r *GuildRepository) FindGuildIDByName(ctx context.Context, name string) (uint32, error) {
	var guildID uint32
	err := r.store.db.QueryRowContext(ctx, `select fld_id from tbl_guild where fld_name = ?`, name).Scan(&guildID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find guild: %w", err)
	}
	return guildID, nil
}

func (r *GuildRepository) AtWar(ctx context.Context, guildID, otherGuildID uint32, now int64) (bool, error) {
	if guildID == 0 || otherGuildID == 0 || guildID == otherGuildID {
		return false, nil
	}
	if now == 0 {
		now = time.Now().Unix()
	}
	first, second := guildID, otherGuildID
	if first > second {
		first, second = second, first
	}
	var count int
	if err := r.store.db.QueryRowContext(ctx, `select count(*) from tbl_guildwar where fld_guild1 = ? and fld_guild2 = ? and fld_endtime > ?`, first, second, now).Scan(&count); err != nil {
		return false, fmt.Errorf("check guild war: %w", err)
	}
	return count != 0, nil
}

func (r *GuildRepository) LoadWarList(ctx context.Context, guildID uint32, now int64) ([]protocol.SDGuildWar, error) {
	if now == 0 {
		now = time.Now().Unix()
	}
	rows, err := r.store.db.QueryContext(ctx, `
select g.fld_id, g.fld_name, w.fld_endtime
from tbl_guildwar w join tbl_guild g
on g.fld_id = case w.fld_guild1 when ? then w.fld_guild2 else w.fld_guild1 end
where (w.fld_guild1 = ? or w.fld_guild2 = ?) and w.fld_endtime > ?
order by w.fld_endtime, g.fld_id`, guildID, guildID, guildID, now)
	if err != nil {
		return nil, fmt.Errorf("load guild wars: %w", err)
	}
	defer rows.Close()
	var result []protocol.SDGuildWar
	for rows.Next() {
		var war protocol.SDGuildWar
		if err := rows.Scan(&war.GuildID, &war.GuildName, &war.EndTime); err != nil {
			return nil, err
		}
		result = append(result, war)
	}
	return result, rows.Err()
}
