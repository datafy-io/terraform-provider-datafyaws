package datafy

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/terraform-provider-aws/internal/slices"
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
		Filters: []*ec2.Filter{
			{
				Name:   aws.String(fmt.Sprintf("tag:%s", managedByTagKey)),
				Values: aws.StringSlice([]string{managedByTagValue}),
			},
			{
				Name:   aws.String(fmt.Sprintf("tag:%s", sourceVolumeTagKey)),
				Values: aws.StringSlice([]string{sourceVolumeId}),
			},
		},
	}
}

// TagsFrom flattens TagSpecifications for the given resource type into a key/value map.
func TagsFrom(ts []*ec2.TagSpecification, rt string) map[string]string {
	tags := make(map[string]string)
	for _, spec := range ts {
		if aws.StringValue(spec.ResourceType) != rt {
			continue
		}
		for _, t := range spec.Tags {
			tags[aws.StringValue(t.Key)] = aws.StringValue(t.Value)
		}
	}
	return tags
}

func RemoveDatafyTags(tags []*ec2.Tag) []*ec2.Tag {
	return slices.Filter(tags, func(t *ec2.Tag) bool {
		key := aws.StringValue(t.Key)
		return !(strings.HasPrefix(key, tagsPrefix) || (key == managedByTagKey && aws.StringValue(t.Value) == managedByTagValue))
	})
}

// IsManagedByDatafy reports whether the tags mark a resource Datafy created and owns.
func IsManagedByDatafy(tags []*ec2.Tag) bool {
	return slices.Any(tags, func(t *ec2.Tag) bool {
		return aws.StringValue(t.Key) == managedByTagKey && aws.StringValue(t.Value) == managedByTagValue
	})
}

// GetSourceVolumeId returns the ID of the volume a datafy volume replaces from the datafy
// volume's tags, or an empty string if the tags don't name one.
func GetSourceVolumeId(tags []*ec2.Tag) string {
	for _, t := range tags {
		if aws.StringValue(t.Key) == sourceVolumeTagKey {
			return aws.StringValue(t.Value)
		}
	}
	return ""
}

func GetRestoredFromSnapshotId(tags []*ec2.Tag) string {
	for _, t := range tags {
		if aws.StringValue(t.Key) == restoredFromSnapshotIdTagKey {
			return aws.StringValue(t.Value)
		}
	}
	return ""
}

// GetDatafySnapshotId returns the Datafy snapshot ID from a snapshot's tags,
// or an empty string if the snapshot is not a datafied volume snapshot.
func GetDatafySnapshotId(tags []*ec2.Tag) string {
	for _, t := range tags {
		if aws.StringValue(t.Key) == datafySnapshotIdTagKey {
			return aws.StringValue(t.Value)
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
