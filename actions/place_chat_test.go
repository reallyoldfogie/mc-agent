package actions

import (
	"testing"
	"time"
)

// A placement that keeps failing instantly must not chat every time: the
// server kicked the bot for spamming after 11 lines in a few milliseconds.
func TestPlaceErrorChatIsRateLimited(t *testing.T) {
	placeErrorChatLast = time.Time{}
	start := time.Now()
	if !placeErrorChatAllowed(start) {
		t.Fatal("the first error must be allowed")
	}
	for i := 1; i < 50; i++ {
		if placeErrorChatAllowed(start.Add(time.Duration(i) * 10 * time.Millisecond)) {
			t.Fatalf("error %d, %v after the first, was allowed", i, time.Duration(i)*10*time.Millisecond)
		}
	}
	if !placeErrorChatAllowed(start.Add(placeErrorChatEvery + time.Millisecond)) {
		t.Fatal("an error after the gap must be allowed again")
	}
}
