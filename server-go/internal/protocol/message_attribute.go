package protocol

import "fmt"

// MessageKind matches the wire-level message classes defined by msgf::MsgAttribute.
type MessageKind uint8

const (
	// MessageEmpty has no payload.
	MessageEmpty MessageKind = iota

	// MessageFixedCompressed has a known uncompressed payload size and uses XOR compression.
	MessageFixedCompressed

	// MessageFixedRaw has a known payload size and is sent unchanged.
	MessageFixedRaw

	// MessageVariable has a variable payload size and is sent unchanged.
	MessageVariable
)

// MessageAttribute describes the framing rules for one legacy message code.
type MessageAttribute struct {
	Name    string
	Kind    MessageKind
	DataLen int
}

// ValidatePayload checks a payload against the fixed-size message contract.
func (a MessageAttribute) ValidatePayload(payload []byte) error {
	switch a.Kind {
	case MessageEmpty:
		if len(payload) != 0 {
			return fmt.Errorf("%s expects an empty payload, got %d bytes", a.Name, len(payload))
		}
	case MessageFixedCompressed, MessageFixedRaw:
		if len(payload) != a.DataLen {
			return fmt.Errorf("%s expects %d payload bytes, got %d", a.Name, a.DataLen, len(payload))
		}
	case MessageVariable:
	// Variable messages may be empty, matching msgf::MsgBase::checkData.
	default:
		return fmt.Errorf("%s has invalid message kind %d", a.Name, a.Kind)
	}
	return nil
}

// ClientMessageAttribute returns metadata for a masked client message code.
func ClientMessageAttribute(code CMType) (MessageAttribute, bool) {
	attr, ok := clientMessageAttributes[code]
	return attr, ok
}

// ServerMessageAttribute returns metadata for a masked server message code.
func ServerMessageAttribute(code SMType) (MessageAttribute, bool) {
	attr, ok := serverMessageAttributes[code]
	return attr, ok
}

func clientMessageAttributeForHead(head uint8) (MessageAttribute, bool) {
	return ClientMessageAttribute(CMType(head & 0x7f))
}

func serverMessageAttributeForHead(head uint8) (MessageAttribute, bool) {
	return ServerMessageAttribute(SMType(head & 0x7f))
}
