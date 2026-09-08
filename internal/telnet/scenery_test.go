package telnet

import "testing"

func TestSceneryTakeReplies(t *testing.T) {
	if sceneryTakeReply("Town Square", "the FOUNTAIN") == "" {
		t.Fatal("missing scenery reply")
	}
	if sceneryTakeReply("Market Lane", "fountain") != "" {
		t.Fatal("scenery belongs to another room")
	}
	if sceneryTakeReply("Town Square", "sword") != "" {
		t.Fatal("portable item treated as scenery")
	}
	if sceneryTakeReply("Town Square", "lamppost") == "" {
		t.Fatal("missing lamppost reply")
	}
}
