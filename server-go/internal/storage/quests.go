package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

var questFieldPattern = regexp.MustCompile(`^fld_[a-z0-9_]+$`)

// QuestRepository owns dynamically-created quest state tables.
type QuestRepository struct {
	store *Store
}

func NewQuestRepository(store *Store) *QuestRepository { return &QuestRepository{store: store} }

func validateQuestTableName(name string) error {
	if name == "" || len(name) > 128 {
		return errors.New("invalid quest table name")
	}
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return fmt.Errorf("invalid quest table name %q", name)
		}
	}
	return nil
}

func validateQuestFieldName(name string) error {
	if !questFieldPattern.MatchString(name) || name == "fld_dbid" || name == "fld_timestamp" {
		return fmt.Errorf("invalid quest field name %q", name)
	}
	return nil
}

// CreateTable creates one quest table if it does not exist.
func (r *QuestRepository) CreateTable(ctx context.Context, table string) error {
	if err := validateQuestTableName(table); err != nil {
		return err
	}
	query := fmt.Sprintf(`
create table if not exists %s(
fld_dbid      integer not null,
fld_timestamp integer not null,
fld_states    blob null,
fld_flags     blob null,
fld_team      blob null,
fld_vars      blob null,
fld_desp      blob null,
fld_context   blob null,
foreign key (fld_dbid) references tbl_char(fld_dbid) on delete cascade,
primary key (fld_dbid)
)`, table)
	if _, err := r.store.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create quest table %s: %w", table, err)
	}
	return nil
}

func (r *QuestRepository) LoadField(ctx context.Context, table string, dbid uint32, field string) (protocol.LuaVar, bool, error) {
	if err := validateQuestTableName(table); err != nil {
		return protocol.LuaVar{}, false, err
	}
	if err := validateQuestFieldName(field); err != nil {
		return protocol.LuaVar{}, false, err
	}
	query := fmt.Sprintf(`select %s from %s where fld_dbid = ? and %s is not null`, field, table, field)
	var blob []byte
	err := r.store.db.QueryRowContext(ctx, query, dbid).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.NewLuaNilVar(), false, nil
	}
	if err != nil {
		return protocol.LuaVar{}, false, fmt.Errorf("load quest field: %w", err)
	}
	var value protocol.LuaVar
	if err := UnmarshalCerealBlob(blob, &value); err != nil {
		return protocol.LuaVar{}, false, fmt.Errorf("decode quest field: %w", err)
	}
	return value, true, nil
}

// UpdateFields writes only whitelisted quest fields. Nil values are stored as SQL NULL.
func (r *QuestRepository) UpdateFields(ctx context.Context, table string, dbid uint32, fields map[string]*protocol.LuaVar, replace bool) error {
	if err := validateQuestTableName(table); err != nil {
		return err
	}
	if len(fields) == 0 {
		return errors.New("no quest fields")
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		if err := validateQuestFieldName(name); err != nil {
			return err
		}
		names = append(names, name)
	}
	sort.Strings(names)

	columns := make([]string, 0, len(names))
	values := make([]string, 0, len(names))
	updates := make([]string, 0, len(names))
	for _, name := range names {
		columns = append(columns, name)
		values = append(values, "?")
		updates = append(updates, name+"=excluded."+name)
	}
	timestamp := time.Now().UnixNano()
	verb := "insert into"
	suffix := " on conflict(fld_dbid) do update set fld_timestamp=excluded.fld_timestamp, " + strings.Join(updates, ", ")
	if replace {
		verb = "replace into"
		suffix = ""
	}
	query := fmt.Sprintf(`%s %s(fld_dbid, fld_timestamp, %s) values(?, ?, %s)%s`,
		verb, table, strings.Join(columns, ", "), strings.Join(values, ", "), suffix)

	args := make([]any, 0, len(names)+2)
	args = append(args, dbid, timestamp)
	for _, name := range names {
		value := fields[name]
		if value == nil || value.Kind == protocol.LuaVarNil {
			args = append(args, nil)
			continue
		}
		blob, err := MarshalCerealBlob(*value)
		if err != nil {
			return fmt.Errorf("encode quest field %s: %w", name, err)
		}
		args = append(args, blob)
	}
	if _, err := r.store.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("update quest fields: %w", err)
	}
	return nil
}

func (r *QuestRepository) Delete(ctx context.Context, table string, dbid uint32) error {
	if err := validateQuestTableName(table); err != nil {
		return err
	}
	query := fmt.Sprintf(`delete from %s where fld_dbid = ?`, table)
	if _, err := r.store.db.ExecContext(ctx, query, dbid); err != nil {
		return fmt.Errorf("delete quest row: %w", err)
	}
	return nil
}
