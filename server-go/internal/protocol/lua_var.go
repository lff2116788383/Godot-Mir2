package protocol

import "fmt"

// LuaVarKind mirrors the variant index of luaf::luaVar.
type LuaVarKind int32

const (
	LuaVarNil LuaVarKind = iota
	LuaVarArray
	LuaVarTable
	LuaVarInteger
	LuaVarNumber
	LuaVarBool
	LuaVarString
)

// LuaVarWrapper mirrors luaf::luaVarWrapper.
// Valid=false is the unique_ptr==nullptr nil wrapper.
// Valid=true carries one concrete variant value.
type LuaVarWrapper struct {
	Valid bool
	Value LuaVar
}

// LuaTableEntry preserves one table entry. C++ uses unordered_map, so the Go
// representation preserves wire order instead of imposing map key ordering.
type LuaTableEntry struct {
	Key   LuaVarWrapper
	Value LuaVarWrapper
}

// LuaVar mirrors luaf::luaVar's recursive cereal representation.
type LuaVar struct {
	Kind LuaVarKind

	NilPlaceholder uint8
	Array          []LuaVarWrapper
	Table          []LuaTableEntry

	Integer int64
	Number  float64
	Boolean bool
	String  string
}

func NewLuaNilVar() LuaVar {
	return LuaVar{Kind: LuaVarNil}
}

func NewLuaArrayVar(values ...LuaVarWrapper) LuaVar {
	return LuaVar{Kind: LuaVarArray, Array: append([]LuaVarWrapper(nil), values...)}
}

func NewLuaTableVar(entries ...LuaTableEntry) LuaVar {
	return LuaVar{Kind: LuaVarTable, Table: append([]LuaTableEntry(nil), entries...)}
}

func NewLuaIntegerVar(value int64) LuaVar  { return LuaVar{Kind: LuaVarInteger, Integer: value} }
func NewLuaNumberVar(value float64) LuaVar { return LuaVar{Kind: LuaVarNumber, Number: value} }
func NewLuaBoolVar(value bool) LuaVar      { return LuaVar{Kind: LuaVarBool, Boolean: value} }
func NewLuaStringVar(value string) LuaVar  { return LuaVar{Kind: LuaVarString, String: value} }
func WrapLuaVar(value LuaVar) LuaVarWrapper {
	return LuaVarWrapper{Valid: true, Value: value}
}

func (v LuaVarWrapper) MarshalPortable(writer *PortableWriter) error {
	writer.Bool(v.Valid)
	if v.Valid {
		return v.Value.MarshalPortable(writer)
	}
	return writer.Err()
}

func (v *LuaVarWrapper) UnmarshalPortable(reader *PortableReader) error {
	v.Valid = reader.Bool()
	if !v.Valid {
		v.Value = LuaVar{}
		return reader.Err()
	}
	return v.Value.UnmarshalPortable(reader)
}

func (v LuaVar) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(int32(v.Kind))
	switch v.Kind {
	case LuaVarNil:
		writer.Uint8(v.NilPlaceholder)
	case LuaVarArray:
		writer.SizeTag(uint64(len(v.Array)))
		for _, value := range v.Array {
			if err := value.MarshalPortable(writer); err != nil {
				return err
			}
		}
	case LuaVarTable:
		writer.SizeTag(uint64(len(v.Table)))
		for _, entry := range v.Table {
			if err := entry.Key.MarshalPortable(writer); err != nil {
				return err
			}
			if err := entry.Value.MarshalPortable(writer); err != nil {
				return err
			}
		}
	case LuaVarInteger:
		writer.Int64(v.Integer)
	case LuaVarNumber:
		writer.Float64(v.Number)
	case LuaVarBool:
		writer.Bool(v.Boolean)
	case LuaVarString:
		writer.String(v.String)
	default:
		return fmt.Errorf("invalid LuaVar kind %d", v.Kind)
	}
	return writer.Err()
}

func (v *LuaVar) UnmarshalPortable(reader *PortableReader) error {
	v.Kind = LuaVarKind(reader.Int32())
	switch v.Kind {
	case LuaVarNil:
		v.NilPlaceholder = reader.Uint8()
	case LuaVarArray:
		count := readPortableCount(reader, 1<<20, "Lua array")
		if reader.Err() != nil {
			return reader.Err()
		}
		if count == 0 {
			v.Array = nil
		} else {
			v.Array = make([]LuaVarWrapper, count)
		}
		for i := range v.Array {
			if err := v.Array[i].UnmarshalPortable(reader); err != nil {
				return err
			}
		}
	case LuaVarTable:
		count := readPortableCount(reader, 1<<20, "Lua table")
		if reader.Err() != nil {
			return reader.Err()
		}
		if count == 0 {
			v.Table = nil
		} else {
			v.Table = make([]LuaTableEntry, count)
		}
		for i := range v.Table {
			if err := v.Table[i].Key.UnmarshalPortable(reader); err != nil {
				return err
			}
			if err := v.Table[i].Value.UnmarshalPortable(reader); err != nil {
				return err
			}
		}
	case LuaVarInteger:
		v.Integer = reader.Int64()
	case LuaVarNumber:
		v.Number = reader.Float64()
	case LuaVarBool:
		v.Boolean = reader.Bool()
	case LuaVarString:
		v.String = reader.String()
	default:
		return fmt.Errorf("invalid LuaVar kind %d", v.Kind)
	}
	return reader.Err()
}
