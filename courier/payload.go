// Package courier implements the Go-side "item_transfer:*" plugin-message
// protocol described in mc-item-transfer-mod's docs/protocol.md. This file
// covers just the wire encoding of that protocol's 8 payload types - turning
// each into a (channel string, []byte) pair for agent.SendPluginMessage, and
// back for a models.PluginMessageCallback. It does not parse or make
// decisions based on a JWT's contents anywhere; jwt/jti fields are opaque
// strings, per that doc's "stateless black-box courier" requirement.
package courier

import (
	"bytes"
	"fmt"

	pk "github.com/Tnze/go-mc/net/packet"
)

// Channel identifiers, namespaced "item_transfer:*" per docs/protocol.md §2.
const (
	ChannelHandshake = "item_transfer:handshake"
	ChannelRequest   = "item_transfer:request"
	ChannelHold      = "item_transfer:hold"
	ChannelClaim     = "item_transfer:claim"
	ChannelPromised  = "item_transfer:promised"
	ChannelCommit    = "item_transfer:commit"
	ChannelAbort     = "item_transfer:abort"
	ChannelError     = "item_transfer:error"
)

// writeString appends a VarInt-length-prefixed UTF-8 string, matching both
// go-mc's pk.String wire format and the Java side's ByteBufCodecs.STRING_UTF8
// / PayloadCodecs.JWT_STRING (which differ only in max-length bound, not
// wire shape).
func writeString(buf *bytes.Buffer, s string) error {
	_, err := pk.String(s).WriteTo(buf)
	return err
}

// readString reads one VarInt-length-prefixed UTF-8 string from r, returning
// how many bytes remain unread afterward.
func readString(r *bytes.Reader) (string, error) {
	var s pk.String
	if _, err := s.ReadFrom(r); err != nil {
		return "", err
	}
	return string(s), nil
}

// writeVarInt appends a VarInt, matching both go-mc's pk.VarInt wire format
// and the Java side's ByteBufCodecs.VAR_INT.
func writeVarInt(buf *bytes.Buffer, v int32) error {
	_, err := pk.VarInt(v).WriteTo(buf)
	return err
}

func readVarInt(r *bytes.Reader) (int32, error) {
	var v pk.VarInt
	if _, err := v.ReadFrom(r); err != nil {
		return 0, err
	}
	return int32(v), nil
}

// HandshakePayload is item_transfer:handshake (C->S then S->C): protocol
// version exchange, sent once per connection before any transfer, on every
// connected server.
type HandshakePayload struct {
	ProtocolVersion int32
}

func (p HandshakePayload) Channel() string { return ChannelHandshake }

func (p HandshakePayload) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeVarInt(&buf, p.ProtocolVersion); err != nil {
		return nil, fmt.Errorf("encode HandshakePayload: %w", err)
	}
	return buf.Bytes(), nil
}

func DecodeHandshakePayload(data []byte) (HandshakePayload, error) {
	r := bytes.NewReader(data)
	v, err := readVarInt(r)
	if err != nil {
		return HandshakePayload{}, fmt.Errorf("decode HandshakePayload: %w", err)
	}
	return HandshakePayload{ProtocolVersion: v}, nil
}

// RequestPayload is item_transfer:request (C->S): player wants to transfer
// the item in the given inventory slot ("0"-"35", main inventory + hotbar
// only). Sent to the source server.
type RequestPayload struct {
	SlotID string
}

func (p RequestPayload) Channel() string { return ChannelRequest }

func (p RequestPayload) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeString(&buf, p.SlotID); err != nil {
		return nil, fmt.Errorf("encode RequestPayload: %w", err)
	}
	return buf.Bytes(), nil
}

func DecodeRequestPayload(data []byte) (RequestPayload, error) {
	r := bytes.NewReader(data)
	slotID, err := readString(r)
	if err != nil {
		return RequestPayload{}, fmt.Errorf("decode RequestPayload: %w", err)
	}
	return RequestPayload{SlotID: slotID}, nil
}

// HoldPayload is item_transfer:hold (S->C): the source server has staged the
// item and attaches the signed JWT item passport plus its jti.
type HoldPayload struct {
	JWT string
	JTI string
}

func (p HoldPayload) Channel() string { return ChannelHold }

func (p HoldPayload) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeString(&buf, p.JWT); err != nil {
		return nil, fmt.Errorf("encode HoldPayload: %w", err)
	}
	if err := writeString(&buf, p.JTI); err != nil {
		return nil, fmt.Errorf("encode HoldPayload: %w", err)
	}
	return buf.Bytes(), nil
}

func DecodeHoldPayload(data []byte) (HoldPayload, error) {
	r := bytes.NewReader(data)
	jwt, err := readString(r)
	if err != nil {
		return HoldPayload{}, fmt.Errorf("decode HoldPayload: %w", err)
	}
	jti, err := readString(r)
	if err != nil {
		return HoldPayload{}, fmt.Errorf("decode HoldPayload: %w", err)
	}
	return HoldPayload{JWT: jwt, JTI: jti}, nil
}

// ClaimPayload is item_transfer:claim (C->S): the client forwards the raw JWT
// text, unmodified and unparsed, to the destination server.
type ClaimPayload struct {
	JWT string
}

func (p ClaimPayload) Channel() string { return ChannelClaim }

func (p ClaimPayload) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeString(&buf, p.JWT); err != nil {
		return nil, fmt.Errorf("encode ClaimPayload: %w", err)
	}
	return buf.Bytes(), nil
}

func DecodeClaimPayload(data []byte) (ClaimPayload, error) {
	r := bytes.NewReader(data)
	jwt, err := readString(r)
	if err != nil {
		return ClaimPayload{}, fmt.Errorf("decode ClaimPayload: %w", err)
	}
	return ClaimPayload{JWT: jwt}, nil
}

// PromisedPayload is item_transfer:promised (S->C): the destination server
// verified the passport, delivered the item, and recorded the redemption.
type PromisedPayload struct {
	JTI string
}

func (p PromisedPayload) Channel() string { return ChannelPromised }

func (p PromisedPayload) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeString(&buf, p.JTI); err != nil {
		return nil, fmt.Errorf("encode PromisedPayload: %w", err)
	}
	return buf.Bytes(), nil
}

func DecodePromisedPayload(data []byte) (PromisedPayload, error) {
	r := bytes.NewReader(data)
	jti, err := readString(r)
	if err != nil {
		return PromisedPayload{}, fmt.Errorf("decode PromisedPayload: %w", err)
	}
	return PromisedPayload{JTI: jti}, nil
}

// CommitPayload is item_transfer:commit (C->S): the client relays the
// destination server's confirmation back to the source server, which may now
// purge the staged item.
type CommitPayload struct {
	JTI string
}

func (p CommitPayload) Channel() string { return ChannelCommit }

func (p CommitPayload) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeString(&buf, p.JTI); err != nil {
		return nil, fmt.Errorf("encode CommitPayload: %w", err)
	}
	return buf.Bytes(), nil
}

func DecodeCommitPayload(data []byte) (CommitPayload, error) {
	r := bytes.NewReader(data)
	jti, err := readString(r)
	if err != nil {
		return CommitPayload{}, fmt.Errorf("decode CommitPayload: %w", err)
	}
	return CommitPayload{JTI: jti}, nil
}

// AbortPayload is item_transfer:abort (C->S or S->C): explicit cancellation
// of an in-flight transfer before commit - triggers the same return-to-owner
// path as the server's automatic expiry sweep.
type AbortPayload struct {
	JTI    string
	Reason string
}

func (p AbortPayload) Channel() string { return ChannelAbort }

func (p AbortPayload) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeString(&buf, p.JTI); err != nil {
		return nil, fmt.Errorf("encode AbortPayload: %w", err)
	}
	if err := writeString(&buf, p.Reason); err != nil {
		return nil, fmt.Errorf("encode AbortPayload: %w", err)
	}
	return buf.Bytes(), nil
}

func DecodeAbortPayload(data []byte) (AbortPayload, error) {
	r := bytes.NewReader(data)
	jti, err := readString(r)
	if err != nil {
		return AbortPayload{}, fmt.Errorf("decode AbortPayload: %w", err)
	}
	reason, err := readString(r)
	if err != nil {
		return AbortPayload{}, fmt.Errorf("decode AbortPayload: %w", err)
	}
	return AbortPayload{JTI: jti, Reason: reason}, nil
}

// ErrorPayload is item_transfer:error (S->C): any phase failed. JTI may be
// empty if the failure happened before a transfer id was known (e.g. an
// invalid slot on "request"). See docs/protocol.md §5 for the code enum.
type ErrorPayload struct {
	JTI     string
	Code    string
	Message string
}

func (p ErrorPayload) Channel() string { return ChannelError }

func (p ErrorPayload) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeString(&buf, p.JTI); err != nil {
		return nil, fmt.Errorf("encode ErrorPayload: %w", err)
	}
	if err := writeString(&buf, p.Code); err != nil {
		return nil, fmt.Errorf("encode ErrorPayload: %w", err)
	}
	if err := writeString(&buf, p.Message); err != nil {
		return nil, fmt.Errorf("encode ErrorPayload: %w", err)
	}
	return buf.Bytes(), nil
}

func DecodeErrorPayload(data []byte) (ErrorPayload, error) {
	r := bytes.NewReader(data)
	jti, err := readString(r)
	if err != nil {
		return ErrorPayload{}, fmt.Errorf("decode ErrorPayload: %w", err)
	}
	code, err := readString(r)
	if err != nil {
		return ErrorPayload{}, fmt.Errorf("decode ErrorPayload: %w", err)
	}
	message, err := readString(r)
	if err != nil {
		return ErrorPayload{}, fmt.Errorf("decode ErrorPayload: %w", err)
	}
	return ErrorPayload{JTI: jti, Code: code, Message: message}, nil
}
