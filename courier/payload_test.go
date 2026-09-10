package courier

import (
	"encoding/hex"
	"testing"
)

func TestHandshakePayloadRoundTrip(t *testing.T) {
	want := HandshakePayload{ProtocolVersion: 1}
	data, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeHandshakePayload(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRequestPayloadRoundTrip(t *testing.T) {
	want := RequestPayload{SlotID: "17"}
	data, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeRequestPayload(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestHoldPayloadRoundTrip(t *testing.T) {
	want := HoldPayload{JWT: "eyJhbGciOiJFZERTQSJ9.payload.sig", JTI: "8f14e45f-ceea-4d95-a3a8-1c2b6f5d7e11"}
	data, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeHoldPayload(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestClaimPayloadRoundTrip(t *testing.T) {
	want := ClaimPayload{JWT: "some.jwt.text"}
	data, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeClaimPayload(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPromisedPayloadRoundTrip(t *testing.T) {
	want := PromisedPayload{JTI: "some-jti"}
	data, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodePromisedPayload(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCommitPayloadRoundTrip(t *testing.T) {
	want := CommitPayload{JTI: "some-jti"}
	data, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeCommitPayload(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestAbortPayloadRoundTrip(t *testing.T) {
	want := AbortPayload{JTI: "some-jti", Reason: "client timeout"}
	data, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeAbortPayload(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestErrorPayloadRoundTrip(t *testing.T) {
	want := ErrorPayload{JTI: "", Code: "INVALID_SLOT", Message: "slot 99 is out of range"}
	data, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeErrorPayload(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// TestChannelNamesMatchProtocolDoc pins the channel constants against
// mc-item-transfer-mod/docs/protocol.md §2's channel table, so a rename on
// either side of the wire fails loudly here instead of silently at runtime.
func TestChannelNamesMatchProtocolDoc(t *testing.T) {
	cases := map[string]string{
		ChannelHandshake: "item_transfer:handshake",
		ChannelRequest:   "item_transfer:request",
		ChannelHold:      "item_transfer:hold",
		ChannelClaim:     "item_transfer:claim",
		ChannelPromised:  "item_transfer:promised",
		ChannelCommit:    "item_transfer:commit",
		ChannelAbort:     "item_transfer:abort",
		ChannelError:     "item_transfer:error",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("channel constant %q, want %q", got, want)
		}
	}
}

// TestRequestPayloadWireFormat pins the encoding to Minecraft's actual wire
// format (VarInt length prefix + UTF-8 bytes), not just internal
// self-consistency between Encode/Decode - a bug in both would otherwise
// still pass a pure round-trip test. "17" is 2 bytes, so the VarInt length
// prefix is a single 0x02 byte.
func TestRequestPayloadWireFormat(t *testing.T) {
	data, err := RequestPayload{SlotID: "17"}.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want := "023137" // 0x02 (VarInt len=2), '1' (0x31), '7' (0x37)
	if got := hex.EncodeToString(data); got != want {
		t.Fatalf("wire bytes = %s, want %s", got, want)
	}
}

// TestHandshakePayloadWireFormat pins HandshakePayload's VarInt encoding.
func TestHandshakePayloadWireFormat(t *testing.T) {
	data, err := HandshakePayload{ProtocolVersion: 300}.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	// 300 = 0b1_00101100 -> VarInt bytes: 0xAC 0x02
	want := "ac02"
	if got := hex.EncodeToString(data); got != want {
		t.Fatalf("wire bytes = %s, want %s", got, want)
	}
}
