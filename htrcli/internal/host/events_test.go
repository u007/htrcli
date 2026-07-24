package host

import "testing"

func TestEventStoreIngestAndRead(t *testing.T) {
	es := NewEventStore()
	es.Ingest(1, "console", []Event{
		{Kind: "console", Timestamp: 1000, Data: []byte(`{"level":"log","args":["a"]}`)},
		{Kind: "console", Timestamp: 1001, Data: []byte(`{"level":"log","args":["b"]}`)},
	})

	entries, dropped, oldest := es.Read(1, "console", 0)
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	if entries[0].Seq != 1 || entries[1].Seq != 2 {
		t.Fatalf("want seq 1,2, got %d,%d", entries[0].Seq, entries[1].Seq)
	}
	if dropped != 0 || oldest != 1 {
		t.Fatalf("want dropped=0 oldest=1, got dropped=%d oldest=%d", dropped, oldest)
	}
}

func TestEventStoreEvictionReportsDroppedPerCursor(t *testing.T) {
	es := NewEventStore()
	es.capOverride = 3
	for i := 0; i < 5; i++ {
		es.Ingest(1, "console", []Event{{Kind: "console", Timestamp: int64(i), Data: []byte(`{}`)}})
	}

	entries, dropped, oldest := es.Read(1, "console", 0)
	if len(entries) != 3 || entries[0].Seq != 3 {
		t.Fatalf("want 3 entries starting at seq 3, got %+v", entries)
	}
	if dropped != 2 {
		t.Fatalf("want dropped=2, got %d", dropped)
	}
	if oldest != 3 {
		t.Fatalf("want oldestAvailableSeq=3, got %d", oldest)
	}

	entries2, dropped2, _ := es.Read(1, "console", 3)
	if len(entries2) != 2 || dropped2 != 0 {
		t.Fatalf("want 2 entries dropped=0, got %d entries dropped=%d", len(entries2), dropped2)
	}
}

func TestEventStoreSeparatesTabsAndKinds(t *testing.T) {
	es := NewEventStore()
	es.Ingest(1, "console", []Event{{Kind: "console", Timestamp: 1, Data: []byte(`{}`)}})
	es.Ingest(2, "console", []Event{{Kind: "console", Timestamp: 1, Data: []byte(`{}`)}})
	es.Ingest(1, "network", []Event{{Kind: "network", Timestamp: 1, Data: []byte(`{}`)}})

	if entries, _, _ := es.Read(1, "console", 0); len(entries) != 1 {
		t.Fatalf("tab 1 console: want 1 entry, got %d", len(entries))
	}
	if entries, _, _ := es.Read(2, "console", 0); len(entries) != 1 {
		t.Fatalf("tab 2 console: want 1 entry, got %d", len(entries))
	}
	if entries, _, _ := es.Read(1, "network", 0); len(entries) != 1 {
		t.Fatalf("tab 1 network: want 1 entry, got %d", len(entries))
	}
}

func TestEventStoreGenerationSetOnce(t *testing.T) {
	es := NewEventStore()
	g1 := es.Generation()
	es2 := NewEventStore()
	g2 := es2.Generation()
	if g1 == g2 {
		t.Fatalf("expected distinct generations across daemon instances, got %d == %d", g1, g2)
	}
}
