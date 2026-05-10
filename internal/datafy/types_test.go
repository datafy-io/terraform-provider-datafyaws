package datafy

import (
	"encoding/json"
	"testing"
)

func TestVolumeUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name               string
		jsonData           []byte
		expectedVolID      string
		expectedSizeBytes  uint64
		expectedIops       int32
		expectedThroughput int32
	}{
		{
			name: "Managed volume",
			jsonData: []byte(`{
				"volumeId": "vol-12345",
				"hasSource": false,
				"isManaged": true,
				"isDatafied": true,
				"replacedBy": "",
				"sizeBytes": 107374182400,
				"iops": 3000,
				"throughput": 125
			}`),
			expectedVolID:      "vol-12345",
			expectedSizeBytes:  107374182400,
			expectedIops:       3000,
			expectedThroughput: 125,
		},
		{
			name: "Unmanaged volume with replacement",
			jsonData: []byte(`{
				"volumeId": "vol-67890",
				"hasSource": true,
				"isManaged": false,
				"isDatafied": false,
				"replacedBy": "vol-99999",
				"sizeBytes": 214748364800,
				"iops": 6000,
				"throughput": 250
			}`),
			expectedVolID:      "vol-67890",
			expectedSizeBytes:  214748364800,
			expectedIops:       6000,
			expectedThroughput: 250,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v Volume
			if err := json.Unmarshal(tt.jsonData, &v); err != nil {
				t.Fatalf("UnmarshalJSON failed: %v", err)
			}

			if v.Volume == nil || v.Volume.VolumeId == nil || *v.Volume.VolumeId != tt.expectedVolID {
				gotVolID := ""
				if v.Volume != nil && v.Volume.VolumeId != nil {
					gotVolID = *v.Volume.VolumeId
				}
				t.Errorf("Expected VolumeId to be '%s', got '%s'", tt.expectedVolID, gotVolID)
			}

			if v.Size != nil && *v.Size != int32(tt.expectedSizeBytes/1024/1024/1024) {
				t.Errorf("Expected SizeBytes to be %d, got %d", tt.expectedSizeBytes, v.Size)
			}
			if v.Iops != nil && *v.Iops != tt.expectedIops {
				t.Errorf("Expected Iops to be %d, got %d", tt.expectedIops, v.Iops)
			}
			if v.Throughput != nil && *v.Throughput != tt.expectedThroughput {
				t.Errorf("Expected Throughput to be %d, got %d", tt.expectedThroughput, v.Throughput)
			}
		})
	}
}
