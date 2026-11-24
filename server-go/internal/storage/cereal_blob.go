package storage

import (
	"fmt"
	"sort"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// IntStringMap is the BLOB payload used by item extAttrList fields.
type IntStringMap map[int32]string

func (m IntStringMap) MarshalPortable(writer *protocol.PortableWriter) error {
	keys := make([]int32, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	writer.SizeTag(uint64(len(keys)))
	for _, key := range keys {
		writer.Int32(key)
		writer.String(m[key])
	}
	return writer.Err()
}

func (m *IntStringMap) UnmarshalPortable(reader *protocol.PortableReader) error {
	count := reader.SizeTag()
	if reader.Err() != nil {
		return reader.Err()
	}
	if count > 1<<20 {
		return fmt.Errorf("item attribute count %d exceeds limit", count)
	}
	if count == 0 {
		*m = nil
		return nil
	}
	result := make(IntStringMap, int(count))
	for i := uint64(0); i < count; i++ {
		key := reader.Int32()
		value := reader.String()
		if reader.Err() != nil {
			return reader.Err()
		}
		result[key] = value
	}
	*m = result
	return nil
}

// MarshalCerealBlob serializes one legacy BLOB using cerealf compatibility.
func MarshalCerealBlob(value protocol.PortableMarshaler) ([]byte, error) {
	return protocol.CerealSerialize(value)
}

// UnmarshalCerealBlob decodes one legacy BLOB.
func UnmarshalCerealBlob(data []byte, value protocol.PortableUnmarshaler) error {
	return protocol.CerealDeserialize(data, value)
}

func marshalExtAttrList(attributes map[int32]string) ([]byte, error) {
	return MarshalCerealBlob(IntStringMap(attributes))
}

func unmarshalExtAttrList(data []byte) (map[int32]string, error) {
	var attributes IntStringMap
	if err := UnmarshalCerealBlob(data, &attributes); err != nil {
		return nil, err
	}
	return map[int32]string(attributes), nil
}
