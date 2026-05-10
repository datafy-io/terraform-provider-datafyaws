package ec2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
)

// newTestDatafyClientForWaiter creates a Datafy client pointed at a mock HTTP server.
func newTestDatafyClientForWaiter(t *testing.T, handler http.HandlerFunc) (datafy.Client, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	client := datafy.NewDatafyClient(server.URL, "test-token")
	return client, server.Close
}

// overrideDatafyWaitTiming replaces the polling delays with short values for the duration of the test.
func overrideDatafyWaitTiming(t *testing.T) {
	t.Helper()
	origDelay, origMin := datafyVolumeModifiedDelay, datafyVolumeModifiedMinTimeout
	datafyVolumeModifiedDelay = 10 * time.Millisecond
	datafyVolumeModifiedMinTimeout = 10 * time.Millisecond
	t.Cleanup(func() {
		datafyVolumeModifiedDelay = origDelay
		datafyVolumeModifiedMinTimeout = origMin
	})
}

func volumeStatusBodyWithAttrs(size int, iops int, throughput int) []byte {
	b, _ := json.Marshal(map[string]any{
		"volumeId":   "vol-abc123",
		"isManaged":  true,
		"sizeBytes":  int64(size) * 1024 * 1024 * 1024,
		"iops":       iops,
		"throughput": throughput,
	})
	return b
}

// TestWaitDatafyVolumeModified_modifyingThenChanged verifies the happy path: volume returns
// old values for the first two polls and then transitions to new values.
func TestWaitDatafyVolumeModified_modifyingThenChanged(t *testing.T) {
	overrideDatafyWaitTiming(t)

	var callCount atomic.Int32
	// Simulate size changing from 10 to 20 after 2 polls
	client, cleanup := newTestDatafyClientForWaiter(t, func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		if count <= 2 {
			w.Write(volumeStatusBodyWithAttrs(10, 100, 200))
		} else {
			w.Write(volumeStatusBodyWithAttrs(20, 100, 200))
		}
	})
	defer cleanup()

	err := waitDatafyVolumeModified(context.Background(), client, "vol-abc123", "size", "10", "20", 5*time.Second)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// Must have polled at least 3 times: 2 old value + 1 new value.
	if n := callCount.Load(); n < 3 {
		t.Errorf("expected at least 3 polls, got %d", n)
	}
}

// TestWaitDatafyVolumeModified_alreadyChanged verifies success when the volume is already
// at the new value on the first poll.
func TestWaitDatafyVolumeModified_alreadyChanged(t *testing.T) {
	overrideDatafyWaitTiming(t)

	var callCount atomic.Int32
	// Already at new value (simulate size=20)
	client, cleanup := newTestDatafyClientForWaiter(t, func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write(volumeStatusBodyWithAttrs(20, 100, 200))
	})
	defer cleanup()

	err := waitDatafyVolumeModified(context.Background(), client, "vol-abc123", "size", "10", "20", 5*time.Second)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if n := callCount.Load(); n < 1 {
		t.Errorf("expected exactly 1 poll, got %d", n)
	}
}

// TestWaitDatafyVolumeModified_timeout verifies that the function returns an error when the
// volume stays in MODIFYING for longer than the provided timeout.
func TestWaitDatafyVolumeModified_timeout(t *testing.T) {
	overrideDatafyWaitTiming(t)

	// Always returns old value (size=10), never transitions
	client, cleanup := newTestDatafyClientForWaiter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(volumeStatusBodyWithAttrs(10, 100, 200))
	})
	defer cleanup()

	err := waitDatafyVolumeModified(context.Background(), client, "vol-abc123", "size", "10", "20", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
}

// TestWaitDatafyVolumeModified_apiError verifies that a GetVolume error is propagated and
// aborts the wait immediately.
func TestWaitDatafyVolumeModified_apiError(t *testing.T) {
	overrideDatafyWaitTiming(t)

	client, cleanup := newTestDatafyClientForWaiter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"message": "internal server error"})
	})
	defer cleanup()

	err := waitDatafyVolumeModified(context.Background(), client, "vol-abc123", "size", "10", "20", 5*time.Second)
	if err == nil {
		t.Fatal("expected an error from the API, got nil")
	}
}
