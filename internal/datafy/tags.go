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

func RemoveDatafyTags(tags []*ec2.Tag) []*ec2.Tag {
	return slices.Filter(tags, func(t *ec2.Tag) bool {
		key := aws.StringValue(t.Key)
		return !(strings.HasPrefix(key, tagsPrefix) || (key == managedByTagKey && aws.StringValue(t.Value) == managedByTagValue))
	})
}
