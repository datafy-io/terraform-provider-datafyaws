package datafy

import (
	"testing"
)

func TestValidateNoDatafyTags(t *testing.T) {
	tests := []struct {
		name    string
		tags    map[string]any
		wantErr bool
	}{
		{
			name:    "empty tags",
			tags:    map[string]any{},
			wantErr: false,
		},
		{
			name:    "non-datafy tags allowed",
			tags:    map[string]any{"Name": "my-volume", "Env": "prod"},
			wantErr: false,
		},
		{
			name:    "datafy prefix rejected",
			tags:    map[string]any{"datafy:source-volume:id": "vol-123"},
			wantErr: true,
		},
		{
			name:    "datafy restored-from-snapshot tag rejected",
			tags:    map[string]any{"datafy:restored-from-snapshot:id": "dsnap-123"},
			wantErr: true,
		},
		{
			name:    "mix of valid and datafy tags rejected",
			tags:    map[string]any{"Name": "my-volume", "datafy:foo": "bar"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNoDatafyTags(tt.tags)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNoDatafyTags() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
