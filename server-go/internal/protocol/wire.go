package protocol

import (
	"encoding/binary"
	"fmt"
	"reflect"
)

type wireMarshaler interface {
	AppendWire(dst []byte) ([]byte, error)
}

type wireUnmarshaler interface {
	ReadWire(src []byte) (int, error)
}

var (
	wireMarshalerType   = reflect.TypeOf((*wireMarshaler)(nil)).Elem()
	wireUnmarshalerType = reflect.TypeOf((*wireUnmarshaler)(nil)).Elem()
)

// AppendWirePayload serializes a packed legacy payload struct.
// Fields are emitted in declaration order with no Go alignment padding.
func AppendWirePayload(dst []byte, payload any) ([]byte, error) {
	if payload == nil {
		return dst, nil
	}
	return appendWireValue(dst, reflect.ValueOf(payload))
}

func appendWireValue(dst []byte, value reflect.Value) ([]byte, error) {
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("nil wire payload of type %s", value.Type())
		}
		value = value.Elem()
	}

	if marshaler, ok := wireMarshalerOf(value); ok {
		return marshaler.AppendWire(dst)
	}

	switch value.Kind() {
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		size := value.Type().Bits() / 8
		var raw [8]byte
		binary.LittleEndian.PutUint64(raw[:], value.Uint())
		return append(dst, raw[:size]...), nil

	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		size := value.Type().Bits() / 8
		var raw [8]byte
		binary.LittleEndian.PutUint64(raw[:], uint64(value.Int()))
		return append(dst, raw[:size]...), nil

	case reflect.Bool:
		if value.Bool() {
			return append(dst, 1), nil
		}
		return append(dst, 0), nil

	case reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			for i := 0; i < value.Len(); i++ {
				dst = append(dst, byte(value.Index(i).Uint()))
			}
			return dst, nil
		}
		for i := 0; i < value.Len(); i++ {
			var err error
			dst, err = appendWireValue(dst, value.Index(i))
			if err != nil {
				return nil, fmt.Errorf("%s[%d]: %w", value.Type(), i, err)
			}
		}
		return dst, nil

	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			fieldInfo := value.Type().Field(i)
			if fieldInfo.PkgPath != "" {
				return nil, fmt.Errorf("cannot encode unexported field %s.%s", value.Type(), fieldInfo.Name)
			}
			var err error
			dst, err = appendWireValue(dst, value.Field(i))
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", value.Type(), fieldInfo.Name, err)
			}
		}
		return dst, nil

	default:
		return nil, fmt.Errorf("unsupported wire type %s", value.Type())
	}
}

func wireMarshalerOf(value reflect.Value) (wireMarshaler, bool) {
	if value.IsValid() && value.Type().Implements(wireMarshalerType) {
		return value.Interface().(wireMarshaler), true
	}
	if value.CanAddr() && value.Addr().Type().Implements(wireMarshalerType) {
		return value.Addr().Interface().(wireMarshaler), true
	}
	return nil, false
}

// ReadWirePayload decodes a packed legacy payload into a pointer to a Go struct.
// It returns the number of consumed bytes.
func ReadWirePayload(payload any, src []byte) (int, error) {
	value := reflect.ValueOf(payload)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return 0, fmt.Errorf("payload target must be a non-nil pointer, got %T", payload)
	}
	return readWireValue(value, src)
}

func readWireValue(target reflect.Value, src []byte) (int, error) {
	if target.Kind() == reflect.Pointer {
		if target.IsNil() {
			return 0, fmt.Errorf("nil wire target of type %s", target.Type())
		}
		if unmarshaler, ok := target.Interface().(wireUnmarshaler); ok {
			return unmarshaler.ReadWire(src)
		}
		if target.Elem().Kind() == reflect.Struct && target.Type().Elem().Implements(wireUnmarshalerType) {
			return target.Interface().(wireUnmarshaler).ReadWire(src)
		}
		return readWireValue(target.Elem(), src)
	}

	if target.CanAddr() && target.Addr().Type().Implements(wireUnmarshalerType) {
		return target.Addr().Interface().(wireUnmarshaler).ReadWire(src)
	}

	switch target.Kind() {
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		size := target.Type().Bits() / 8
		if len(src) < size {
			return 0, fmt.Errorf("need %d bytes for %s, got %d", size, target.Type(), len(src))
		}
		var raw [8]byte
		copy(raw[:], src[:size])
		target.SetUint(binary.LittleEndian.Uint64(raw[:]))
		return size, nil

	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		size := target.Type().Bits() / 8
		if len(src) < size {
			return 0, fmt.Errorf("need %d bytes for %s, got %d", size, target.Type(), len(src))
		}
		var raw [8]byte
		copy(raw[:], src[:size])
		target.SetInt(int64(binary.LittleEndian.Uint64(raw[:])))
		return size, nil

	case reflect.Bool:
		if len(src) < 1 {
			return 0, fmt.Errorf("need 1 byte for bool, got 0")
		}
		target.SetBool(src[0] != 0)
		return 1, nil

	case reflect.Array:
		if target.Type().Elem().Kind() == reflect.Uint8 {
			if len(src) < target.Len() {
				return 0, fmt.Errorf("need %d bytes for %s, got %d", target.Len(), target.Type(), len(src))
			}
			reflect.Copy(target, reflect.ValueOf(src[:target.Len()]))
			return target.Len(), nil
		}
		consumed := 0
		for i := 0; i < target.Len(); i++ {
			n, err := readWireValue(target.Index(i), src[consumed:])
			if err != nil {
				return 0, fmt.Errorf("%s[%d]: %w", target.Type(), i, err)
			}
			consumed += n
		}
		return consumed, nil

	case reflect.Struct:
		consumed := 0
		for i := 0; i < target.NumField(); i++ {
			fieldInfo := target.Type().Field(i)
			if fieldInfo.PkgPath != "" {
				return 0, fmt.Errorf("cannot decode unexported field %s.%s", target.Type(), fieldInfo.Name)
			}
			n, err := readWireValue(target.Field(i), src[consumed:])
			if err != nil {
				return 0, fmt.Errorf("%s.%s: %w", target.Type(), fieldInfo.Name, err)
			}
			consumed += n
		}
		return consumed, nil

	default:
		return 0, fmt.Errorf("unsupported wire type %s", target.Type())
	}
}
