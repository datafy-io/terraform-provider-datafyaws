package datafy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
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
	err := client.ModifyVolume("vol-abc123", ModifyVolumeRequest{SizeGb: &sizeGb})

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
	err := client.ModifyVolume("vol-abc123", ModifyVolumeRequest{SizeGb: &sizeGb})

	// Assert error message equals expected message
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	expectedMsg := "volume is not attached to any instance"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestCreateDatafiedVolume_flags(t *testing.T) {
	// The saas reads a volume with no `autoscaling` field as a legacy native autoscaling volume,
	// so all three flags must reach it even when they carry their zero value.
	testCases := map[string]struct {
		request  CreateVolumeRequest
		expected map[string]any
	}{
		"off": {
			request: CreateVolumeRequest{},
			expected: map[string]any{
				"autoscaling":     false,
				"performance":     false,
				"performanceTier": float64(0),
			},
		},
		"autoscaling": {
			request: CreateVolumeRequest{Autoscaling: true},
			expected: map[string]any{
				"autoscaling":     true,
				"performance":     false,
				"performanceTier": float64(0),
			},
		},
		"performance": {
			request: CreateVolumeRequest{Performance: true, PerformanceTier: 4},
			expected: map[string]any{
				"autoscaling":     false,
				"performance":     true,
				"performanceTier": float64(4),
			},
		},
		"both": {
			request: CreateVolumeRequest{Autoscaling: true, Performance: true, PerformanceTier: 8},
			expected: map[string]any{
				"autoscaling":     true,
				"performance":     true,
				"performanceTier": float64(8),
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			client, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					VolumeProperties map[string]any `json:"volumeProperties"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("failed to decode request body: %v", err)
					return
				}

				for key, want := range testCase.expected {
					got, ok := body.VolumeProperties[key]
					if !ok {
						t.Errorf("expected `%s` in the request, got none", key)
						continue
					}
					if got != want {
						t.Errorf("expected `%s` to be %v, got %v", key, want, got)
					}
				}

				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(map[string]string{"volumeId": "vol-abc123"}); err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			}))
			defer cleanup()

			if _, err := client.CreateDatafiedVolume(testCase.request); err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
		})
	}
}

func TestCreateVolumeFromSnapshot_mode(t *testing.T) {
	tests := []struct {
		name string
		req  CreateVolumeFromSnapshotRequest
		// want is the request body's mode fields; an absent key must be absent from the body.
		want map[string]any
	}{
		{
			name: "no mode stated",
			req:  CreateVolumeFromSnapshotRequest{DatafySnapshotId: "dsnap-1"},
			want: map[string]any{},
		},
		{
			name: "pair",
			req:  CreateVolumeFromSnapshotRequest{DatafySnapshotId: "dsnap-1", Autoscaling: aws.Bool(true), Performance: aws.Bool(false)},
			want: map[string]any{"autoscaling": true, "performance": false},
		},
		{
			name: "pinned performance array",
			req:  CreateVolumeFromSnapshotRequest{DatafySnapshotId: "dsnap-1", Autoscaling: aws.Bool(false), Performance: aws.Bool(true), PerformanceTier: 6},
			want: map[string]any{"autoscaling": false, "performance": true, "performanceTier": float64(6)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					VolumeProperties map[string]any `json:"volumeProperties"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decoding request body: %v", err)
				}
				for _, key := range []string{"autoscaling", "performance", "performanceTier"} {
					got, present := body.VolumeProperties[key]
					want, wanted := tt.want[key]
					if present != wanted || got != want {
						t.Errorf("%s: got %v (present %t), want %v (present %t)", key, got, present, want, wanted)
					}
				}

				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(RestoredVolume{VolumeId: "vol-1", VolumeSizeGB: 10}); err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			}))
			defer cleanup()

			if _, err := client.CreateVolumeFromSnapshot(tt.req); err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
		})
	}
}
