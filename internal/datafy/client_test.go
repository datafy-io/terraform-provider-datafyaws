package datafy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (Client, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	client := NewDatafyClient(server.URL, "test-token")
	return client, server.Close
}

func TestModifyVolume_success(t *testing.T) {
	client, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assert method is POST
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Assert path is /api/v1/aws/volumes/vol-abc123/modify
		expectedPath := "/api/v1/aws/volumes/vol-abc123/modify"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		// Decode body and assert fields
		var req modifyVolumeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Assert VolumeSizeGb is non-nil and equals 100
		if req.VolumeSizeGb == nil {
			t.Errorf("VolumeSizeGb should not be nil")
		} else if *req.VolumeSizeGb != 100 {
			t.Errorf("Expected VolumeSizeGb to be 100, got %d", *req.VolumeSizeGb)
		}

		// Assert VolumeIops and VolumeThroughput are nil (omitted from JSON)
		if req.VolumeIops != nil {
			t.Errorf("expected VolumeIops to be nil (omitted), got %v", req.VolumeIops)
		}
		if req.VolumeThroughput != nil {
			t.Errorf("expected VolumeThroughput to be nil (omitted), got %v", req.VolumeThroughput)
		}

		// Return HTTP 200 with {"message":""}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"message": ""}); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer cleanup()

	// Call ModifyVolume
	sizeGb := int32(100)
	err := client.ModifyVolume("vol-abc123", &sizeGb, nil, nil)

	// Assert no error
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestModifyVolume_apiError(t *testing.T) {
	client, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"message": "volume is not attached to any instance"}); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer cleanup()

	// Call ModifyVolume
	sizeGb := int32(100)
	err := client.ModifyVolume("vol-abc123", &sizeGb, nil, nil)

	// Assert error message equals expected message
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	expectedMsg := "volume is not attached to any instance"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}
