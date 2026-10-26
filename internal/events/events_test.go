package events

import (
	"strings"
	"testing"
	"time"
)

func TestRecordValidate(t *testing.T) {
	valid := Record{
		EventID: "e1", AggregateType: "job", AggregateID: "j1", Sequence: 1,
		EventType: "job.created", Producer: "p1", InlinePayload: []byte(`{}`),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	cases := map[string]func(*Record){
		"missing event id":   func(r *Record) { r.EventID = "" },
		"missing aggregate":  func(r *Record) { r.AggregateType = "" },
		"missing event type": func(r *Record) { r.EventType = "" },
		"missing producer":   func(r *Record) { r.Producer = "" },
		"no payload":         func(r *Record) { r.InlinePayload = nil },
		"both payload forms": func(r *Record) {
			r.PayloadDigest = "sha256:" + strings.Repeat("a", 64)
			r.BlobRef = r.PayloadDigest
		},
		"oversized inline": func(r *Record) { r.InlinePayload = make([]byte, MaxInlinePayloadBytes+1) },
		"bad blob ref": func(r *Record) {
			r.InlinePayload = nil
			r.PayloadDigest = "md5:zz"
			r.BlobRef = "md5:zz"
		},
	}
	for name, mutate := range cases {
		r := valid
		mutate(&r)
		if err := r.Validate(); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestDigestGrammar(t *testing.T) {
	if !DigestGrammar("sha256:" + strings.Repeat("a", 64)) {
		t.Fatal("canonical digest must match")
	}
	for _, bad := range []string{
		"", "sha256:", "md5:" + strings.Repeat("a", 64),
		"sha256:" + strings.Repeat("A", 64), // uppercase rejected
		"sha256:" + strings.Repeat("a", 63),
		"sha256:" + strings.Repeat("a", 65),
	} {
		if DigestGrammar(bad) {
			t.Fatalf("%q must not match the grammar", bad)
		}
	}
}

func TestFirstGap(t *testing.T) {
	mk := func(seq uint64) Record { return Record{Sequence: seq, AggregateID: "a", AggregateType: "t"} }
	if _, found := FirstGap([]Record{mk(1), mk(2), mk(3)}); found {
		t.Fatal("gapless run must not report a gap")
	}
	at, found := FirstGap([]Record{mk(1), mk(3)})
	if !found || at != 2 {
		t.Fatalf("gap = %d %v, want 2 true", at, found)
	}
}

func TestCursorRoundtripAndMalformed(t *testing.T) {
	cur, err := EncodeCursor("job", 42, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	stream, seq, err := DecodeCursor(cur)
	if err != nil || stream != "job" || seq != 42 {
		t.Fatalf("roundtrip: %q %d %v", stream, seq, err)
	}
	// Cursors stay opaque: the encoded form is not the stream name.
	if cur == "job" || strings.Contains(strings.Split(cur, ".")[0], "job") {
		t.Fatalf("cursor must be opaque: %s", cur)
	}
	for _, bad := range []string{"", "nope", "aa.zz"} {
		if _, _, err := DecodeCursor(bad); err == nil {
			t.Fatalf("malformed cursor %q must be rejected", bad)
		}
	}
}

func TestSortBySequence(t *testing.T) {
	recs := []Record{
		{AggregateType: "b", AggregateID: "1", Sequence: 1},
		{AggregateType: "a", AggregateID: "2", Sequence: 2},
		{AggregateType: "a", AggregateID: "1", Sequence: 2},
		{AggregateType: "a", AggregateID: "1", Sequence: 1},
	}
	SortBySequence(recs)
	order := ""
	for _, r := range recs {
		order += r.AggregateType + r.AggregateID + string(rune('0'+int(r.Sequence))) + " "
	}
	want := "a11 a12 a22 b11 "
	if order != want {
		t.Fatalf("sort order = %q, want %q", order, want)
	}
}
