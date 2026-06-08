package datafy

import (
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const (
	managedByTagKey    = "Managed-By"
	managedByTagValue  = "Datafy.io"
	sourceVolumeTagKey = "datafy:source-volume:id"
	tagsPrefix         = "datafy:"

	restoredFromSnapshotIdTagKey = "datafy:restored-from-snapshot:id"
	datafySnapshotIdTagKey       = "datafy:snapshot:id"
)

func DescribeDatafiedVolumesInput(sourceVolumeId string) *ec2.DescribeVolumesInput {
	return &ec2.DescribeVolumesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String(fmt.Sprintf("tag:%s", managedByTagKey)),
				Values: []string{managedByTagValue},
			},
			{
				Name:   aws.String(fmt.Sprintf("tag:%s", sourceVolumeTagKey)),
				Values: []string{sourceVolumeId},
			},
		},
	}
}

func RemoveDatafyTags(tags []types.Tag) []types.Tag {
	return slices.DeleteFunc(tags, func(t types.Tag) bool {
		key := aws.ToString(t.Key)
		return strings.HasPrefix(key, tagsPrefix) || (key == managedByTagKey && aws.ToString(t.Value) == managedByTagValue)
	})
}

func GetRestoredFromSnapshotId(tags []types.Tag) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == restoredFromSnapshotIdTagKey {
			return aws.ToString(t.Value)
		}
	}
	return ""
}

// GetDatafySnapshotId returns the Datafy snapshot ID from a snapshot's tags,
// or an empty string if the snapshot is not a datafied volume snapshot.
func GetDatafySnapshotId(tags []types.Tag) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == datafySnapshotIdTagKey {
			return aws.ToString(t.Value)
		}
	}
	return ""
}

func ValidateNoDatafyTags(tags map[string]any) error {
	for key := range tags {
		if strings.HasPrefix(key, tagsPrefix) {
			return fmt.Errorf("tag key %q uses the reserved %q prefix and cannot be set directly. "+
				"Tags with this prefix are managed exclusively by the Datafy platform. "+
				"Remove this tag from your Terraform configuration.", key, tagsPrefix)
		}
	}
	return nil
}
