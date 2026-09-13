package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeChat is a minimal models.Chat implementation that records every
// message it was asked to send, for asserting SendChat splits before
// handing off.
type fakeChat struct {
	sent []string
}

func (f *fakeChat) SendMessage(message string) error {
	f.sent = append(f.sent, message)
	return nil
}

func TestSplitChatMessage_LeavesShortMessageAsOneChunk(t *testing.T) {
	short := "Craft error: craft minecraft:stick: inventory increase never confirmed within 2s"
	require.LessOrEqual(t, len(short), chatMessageMaxLength)
	assert.Equal(t, []string{short}, splitChatMessage(short))
}

func TestSplitChatMessage_LeavesExactlyMaxLengthAsOneChunk(t *testing.T) {
	exact := strings.Repeat("a", chatMessageMaxLength)
	assert.Equal(t, []string{exact}, splitChatMessage(exact))
}

// TestSplitChatMessage_SplitsOverLongMessageWithoutLosingContent guards
// against the exact shape of the real disconnect documented in
// docs/bugs/rare-packet-decode-disconnect/investigation-2026-09-13.md: a
// Craft error message enumerating every valid ingredient-tag alternative
// (e.g. every plank variant) reached 371 characters - 115 over the
// protocol's 256-character limit
// (net.minecraft.network.packet.c2s.play.ChatMessageC2SPacket) - and the
// server disconnected the bot rather than accepting it. Splitting (rather
// than truncating) must deliver every character of the original message,
// just spread across more than one chat send.
func TestSplitChatMessage_SplitsOverLongMessageWithoutLosingContent(t *testing.T) {
	overLong := "Craft error: craft minecraft:stick: missing ingredient (need one of: " +
		"minecraft:oak_planks, minecraft:spruce_planks, minecraft:birch_planks, " +
		"minecraft:jungle_planks, minecraft:acacia_planks, minecraft:dark_oak_planks, " +
		"minecraft:pale_oak_planks, minecraft:crimson_planks, minecraft:warped_planks, " +
		"minecraft:mangrove_planks, minecraft:bamboo_planks, minecraft:cherry_planks)"
	require.Greater(t, len(overLong), chatMessageMaxLength, "test fixture must actually exceed the limit")

	chunks := splitChatMessage(overLong)

	require.Greater(t, len(chunks), 1, "an over-long message must be split into more than one chunk")
	for _, chunk := range chunks {
		assert.LessOrEqual(t, len([]rune(chunk)), chatMessageMaxLength)
		assert.NotEmpty(t, chunk)
	}
	// Rejoining with single spaces must reproduce the original content -
	// splitting breaks at word boundaries (trimming the space consumed by
	// the break itself), so it must never drop or duplicate characters.
	assert.Equal(t, overLong, strings.Join(chunks, " "))
}

func TestSplitChatMessage_NeverSplitsAMultiByteRune(t *testing.T) {
	// A message built almost entirely of 3-byte UTF-8 runes with no spaces,
	// long enough to require a hard split - if splitChatMessage sliced by
	// byte count instead of rune count, this would produce invalid UTF-8.
	overLong := strings.Repeat("世", chatMessageMaxLength+50)
	chunks := splitChatMessage(overLong)

	require.Greater(t, len(chunks), 1)
	var rebuilt strings.Builder
	for _, chunk := range chunks {
		require.True(t, isValidUTF8(chunk))
		assert.LessOrEqual(t, len([]rune(chunk)), chatMessageMaxLength)
		rebuilt.WriteString(chunk)
	}
	assert.Equal(t, overLong, rebuilt.String())
}

func TestSplitChatMessage_SingleWordLongerThanLimitHardSplits(t *testing.T) {
	// No spaces at all: splitChatMessage can't break on a word boundary, so
	// it must fall back to a hard rune-count split rather than looping
	// forever or producing an empty chunk.
	overLong := strings.Repeat("a", chatMessageMaxLength*2+10)
	chunks := splitChatMessage(overLong)

	require.Greater(t, len(chunks), 1)
	var rebuilt strings.Builder
	for _, chunk := range chunks {
		assert.LessOrEqual(t, len([]rune(chunk)), chatMessageMaxLength)
		assert.NotEmpty(t, chunk)
		rebuilt.WriteString(chunk)
	}
	assert.Equal(t, overLong, rebuilt.String())
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}

// TestSendChat_SplitsBeforeFallbackSend verifies the guard is applied at
// the actual SendChat call site (not just the helper in isolation), via the
// fallback bot/msg/chat.go-shaped path (models.Chat), which is what an
// agent without a live version handler/connection falls back to.
func TestSendChat_SplitsBeforeFallbackSend(t *testing.T) {
	fake := &fakeChat{}
	testAgent := &agent{}
	testAgent.SetChat(fake)

	overLong := strings.Repeat("word ", (chatMessageMaxLength*2)/5)
	require.Greater(t, len(overLong), chatMessageMaxLength)
	require.NoError(t, testAgent.SendChat(overLong))

	require.Greater(t, len(fake.sent), 1, "an over-long message must result in more than one SendMessage call")
	for _, sent := range fake.sent {
		assert.LessOrEqual(t, len([]rune(sent)), chatMessageMaxLength)
	}
}

// TestSendChat_ShortMessageSendsExactlyOnce guards against a regression
// where every message - even ones already within the limit - gets wrapped
// or otherwise altered by the new splitting path.
func TestSendChat_ShortMessageSendsExactlyOnce(t *testing.T) {
	fake := &fakeChat{}
	testAgent := &agent{}
	testAgent.SetChat(fake)

	const short = "Hello, world"
	require.NoError(t, testAgent.SendChat(short))

	require.Equal(t, []string{short}, fake.sent)
}
