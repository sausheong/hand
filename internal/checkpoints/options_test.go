package checkpoints

import (
	"context"
	"testing"
)

func TestConfiguredCheckpointScopeAndBounds(t *testing.T) {
	o := Options{Exclude: []string{"private"}, MaxEntries: 10, MaxFileBytes: 10, MaxTotalBytes: 20, MaxSnapshots: 2, MaxStoreBytes: 1 << 20}
	limits, storage, err := o.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	o.Exclude[0] = "other"
	work := t.TempDir()
	put(t, work, "private/token", "secret", 0600)
	put(t, work, "code", "small", 0600)
	snap, err := Capture(context.Background(), work, limits)
	if err != nil || len(snap.Records()) != 1 || len(snap.Omissions()) != 1 {
		t.Fatal(snap, err)
	}
	store, _ := newStore(t, work, storage)
	if err := store.Save(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	put(t, work, "code", "too large for configured limit", 0600)
	if _, err := Capture(context.Background(), work, limits); err == nil {
		t.Fatal("file limit ignored")
	}
	for _, bad := range []Options{{MaxEntries: -1}, {MaxFileBytes: 129 << 20}, {MaxStoreBytes: 9 << 30}, {Exclude: []string{"../secret"}}, {Exclude: []string{"."}}} {
		if _, _, err := bad.Resolve(); err == nil {
			t.Fatal("invalid options accepted", bad)
		}
	}
}
