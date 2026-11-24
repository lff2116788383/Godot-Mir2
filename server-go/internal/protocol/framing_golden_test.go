package protocol

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type framingGoldenFile struct {
	Fixtures map[string]struct {
		Direction  string `json:"direction"`
		ID         uint8  `json:"id"`
		Kind       int    `json:"kind"`
		DataLen    int    `json:"data_len"`
		ResponseID uint64 `json:"response_id"`
		PayloadHex string `json:"payload_hex"`
		PacketHex  string `json:"packet_hex"`
	} `json:"fixtures"`
}

func loadFramingGolden(t testing.TB) framingGoldenFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "framing-golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden framingGoldenFile
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	return golden
}

func TestFramingCppGolden(t *testing.T) {
	t.Parallel()

	golden := loadFramingGolden(t)
	for name, fixture := range golden.Fixtures {
		fixture := fixture
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			payload, err := hex.DecodeString(fixture.PayloadHex)
			if err != nil {
				t.Fatal(err)
			}
			want, err := hex.DecodeString(fixture.PacketHex)
			if err != nil {
				t.Fatal(err)
			}

			var got []byte
			var decoded Packet
			switch fixture.Direction {
			case "client":
				attr, ok := ClientMessageAttribute(CMType(fixture.ID))
				if !ok {
					t.Fatalf("unknown client message %d", fixture.ID)
				}
				if int(attr.Kind) != fixture.Kind {
					t.Fatalf("client kind = %d, fixture = %d", attr.Kind, fixture.Kind)
				}
				if attr.Kind != MessageVariable && attr.DataLen != len(payload) {
					t.Fatalf("client payload size = %d, metadata = %d", len(payload), attr.DataLen)
				}
				got, err = AppendClientPacket(nil, CMType(fixture.ID), payload, fixture.ResponseID)
				if err != nil {
					t.Fatal(err)
				}
				decoded, err = NewPacketDecoder(bytes.NewReader(want)).ReadClientPacket()
				if err != nil {
					t.Fatal(err)
				}

			case "server":
				attr, ok := ServerMessageAttribute(SMType(fixture.ID))
				if !ok {
					t.Fatalf("unknown server message %d", fixture.ID)
				}
				if int(attr.Kind) != fixture.Kind {
					t.Fatalf("server kind = %d, fixture = %d", attr.Kind, fixture.Kind)
				}
				if attr.Kind != MessageVariable && attr.DataLen != len(payload) {
					t.Fatalf("server payload size = %d, metadata = %d", len(payload), attr.DataLen)
				}
				got, err = AppendServerPacket(nil, SMType(fixture.ID), payload, fixture.ResponseID)
				if err != nil {
					t.Fatal(err)
				}
				decoded, err = NewPacketDecoder(bytes.NewReader(want)).ReadServerPacket()
				if err != nil {
					t.Fatal(err)
				}

			default:
				t.Fatalf("unknown direction %q", fixture.Direction)
			}

			if !bytes.Equal(got, want) {
				t.Fatalf("frame mismatch\n got %x\nwant %x", got, want)
			}
			if decoded.Code != fixture.ID {
				t.Fatalf("decoded id = %d, want %d", decoded.Code, fixture.ID)
			}
			if decoded.HasResponse != (fixture.ResponseID != 0) || decoded.ResponseID != fixture.ResponseID {
				t.Fatalf("response = %v/%d, want %d", decoded.HasResponse, decoded.ResponseID, fixture.ResponseID)
			}
			if !bytes.Equal(decoded.Payload, payload) {
				t.Fatalf("decoded payload mismatch\n got %x\nwant %x", decoded.Payload, payload)
			}
		})
	}
}

func FuzzPacketDecoder(f *testing.F) {
	golden := loadFramingGolden(f)
	for _, fixture := range golden.Fixtures {
		packet, err := hex.DecodeString(fixture.PacketHex)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(packet)
	}
	f.Add([]byte{})
	f.Add([]byte{0x7f})
	f.Add([]byte{0x81})

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = NewPacketDecoder(bytes.NewReader(data)).ReadClientPacket()
		_, _ = NewPacketDecoder(bytes.NewReader(data)).ReadServerPacket()
	})
}

func FuzzUvarint(f *testing.F) {
	f.Add([]byte{0})
	f.Add([]byte{0x7f})
	f.Add([]byte{0x80, 0x01})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = readUvarint(bytes.NewReader(data))
	})
}

func FuzzXORDecode(f *testing.F) {
	f.Add([]byte{0x9a, 1, 2, 3, 4}, 8, false)
	f.Add([]byte{0x02, 1, 2, 3, 4, 5, 6, 7, 8}, 16, true)
	f.Fuzz(func(t *testing.T, data []byte, size int, wide bool) {
		if size < 0 || size > 4096 {
			return
		}
		dst := make([]byte, size)
		_, _ = xorDecode(dst, data, wide)
	})
}
