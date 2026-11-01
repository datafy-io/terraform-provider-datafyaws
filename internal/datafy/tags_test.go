package datafy

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
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

func TestGetSourceVolumeId(t *testing.T) {
	tests := []struct {
		name string
		tags []*ec2.Tag
		want string
	}{
		{
			name: "no tags",
			tags: nil,
			want: "",
		},
		{
			name: "not a datafy volume",
			tags: []*ec2.Tag{{Key: aws.String("Name"), Value: aws.String("my-volume")}},
			want: "",
		},
		{
			name: "datafy volume",
			tags: []*ec2.Tag{
				{Key: aws.String("Managed-By"), Value: aws.String("Datafy.io")},
				{Key: aws.String("datafy:source-volume:id"), Value: aws.String("vol-123")},
			},
			want: "vol-123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetSourceVolumeId(tt.tags); got != tt.want {
				t.Errorf("GetSourceVolumeId() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsManagedByDatafy(t *testing.T) {
	tests := []struct {
		name string
		tags []*ec2.Tag
		want bool
	}{
		{
			name: "no tags",
			tags: nil,
			want: false,
		},
		{
			name: "not a datafy volume",
			tags: []*ec2.Tag{{Key: aws.String("Name"), Value: aws.String("my-volume")}},
			want: false,
		},
		{
			name: "other manager",
			tags: []*ec2.Tag{{Key: aws.String("Managed-By"), Value: aws.String("someone-else")}},
			want: false,
		},
		{
			name: "datafy volume",
			tags: []*ec2.Tag{
				{Key: aws.String("Managed-By"), Value: aws.String("Datafy.io")},
				{Key: aws.String("datafy:source-volume:id"), Value: aws.String("vol-123")},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsManagedByDatafy(tt.tags); got != tt.want {
				t.Errorf("IsManagedByDatafy() = %v, want %v", got, tt.want)
			}
		})
	}
}
