package vendor

import (
	"strings"
	"testing"
)

func TestAcquireSyncLock_SecondCallFails(t *testing.T) {
	orig := CacheRootOverride
	CacheRootOverride = t.TempDir()
	defer func() { CacheRootOverride = orig }()

	_, release1, err := AcquireSyncLock()
	if err != nil {
		t.Fatalf("first AcquireSyncLock: unexpected error: %v", err)
	}
	defer release1()

	_, _, err = AcquireSyncLock()
	if err == nil {
		t.Fatal("second concurrent AcquireSyncLock should fail while the first lock is held")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("error should explain another sync is running, got: %v", err)
	}
}

func TestAcquireSyncLock_ReacquireAfterRelease(t *testing.T) {
	orig := CacheRootOverride
	CacheRootOverride = t.TempDir()
	defer func() { CacheRootOverride = orig }()

	_, release1, err := AcquireSyncLock()
	if err != nil {
		t.Fatalf("first AcquireSyncLock: unexpected error: %v", err)
	}
	release1()

	_, release2, err := AcquireSyncLock()
	if err != nil {
		t.Fatalf("AcquireSyncLock after release: unexpected error: %v", err)
	}
	release2()
}
