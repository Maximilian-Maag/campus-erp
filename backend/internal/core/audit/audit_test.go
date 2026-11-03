package audit

import (
	"testing"
	"time"
)

// TestClampLimit: a read of the stream cannot be asked for everything, and a caller that
// passes nothing gets the default rather than an unbounded query.
func TestClampLimit(t *testing.T) {
	cases := map[int]int{
		0:                 defaultEventLimit,
		-1:                defaultEventLimit,
		1:                 1,
		50:                50,
		maxEventLimit:     maxEventLimit,
		maxEventLimit + 1: maxEventLimit,
		100000:            maxEventLimit,
	}
	for in, want := range cases {
		if got := clampLimit(in); got != want {
			t.Errorf("clampLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestAppendChainsEvents(t *testing.T) {
	l := &Log{}
	a := l.Append(Event{Actor: "alice", Action: "create", Subject: "stelle/1"})
	b := l.Append(Event{Actor: "bob", Action: "approve", Subject: "stelle/1"})

	if a.Seq != 1 || b.Seq != 2 {
		t.Fatalf("sequence: got %d,%d want 1,2", a.Seq, b.Seq)
	}
	if a.PrevHash != GenesisHash {
		t.Errorf("first event must chain to genesis")
	}
	if b.PrevHash != a.Hash {
		t.Errorf("second event must chain to the first")
	}
	if a.Hash == b.Hash {
		t.Errorf("distinct events must have distinct hashes")
	}
	if l.Len() != 2 {
		t.Errorf("Len: got %d want 2", l.Len())
	}
}

func TestVerifyAcceptsIntactChain(t *testing.T) {
	l := &Log{}
	for i := 0; i < 5; i++ {
		l.Append(Event{Actor: "a", Action: "x", Subject: "s"})
	}
	if err := l.Verify(); err != nil {
		t.Fatalf("intact chain must verify: %v", err)
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]Event)
	}{
		{"changed actor", func(e []Event) { e[1].Actor = "mallory" }},
		{"changed subject", func(e []Event) { e[2].Subject = "stelle/999" }},
		{"broken predecessor link", func(e []Event) { e[3].PrevHash = GenesisHash }},
		{"renumbered sequence", func(e []Event) { e[2].Seq = 99 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := &Log{}
			for i := 0; i < 4; i++ {
				l.Append(Event{Actor: "a", Action: "x", Subject: "s"})
			}
			tampered := l.Events()
			tc.mutate(tampered)
			l.events = tampered
			if err := l.Verify(); err == nil {
				t.Fatalf("tampering must be detected (%s)", tc.name)
			}
		})
	}
}

func TestHashDependsOnTimestamp(t *testing.T) {
	l := &Log{}
	a := l.Append(Event{Actor: "a", Action: "x", At: time.Unix(0, 0)})
	b := l.Append(Event{Actor: "a", Action: "x", At: time.Unix(1, 0)})
	if a.Hash == b.Hash {
		t.Errorf("hash must depend on the timestamp")
	}
}
