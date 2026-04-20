// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ec2_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/hashicorp/terraform-provider-aws/internal/acctest"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
	tfec2 "github.com/hashicorp/terraform-provider-aws/internal/service/ec2"
	"github.com/hashicorp/terraform-provider-aws/internal/slices"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
)

func TestAccDatafyEC2EBSVolumeAttachment_basic(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	resourceName := "aws_volume_attachment.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeAttachmentDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeAttachmentConfig_basic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeAttachmentExists(ctx, resourceName),
					testAccCheckVolumeExists(ctx, "aws_ebs_volume.test", &v),
				),
			},
			{
				PreConfig: createDatafyVolumeAttachment(ctx, &v, 10),
				Config:    testAccEBSVolumeAttachmentConfig_basic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeAttachmentExists(ctx, resourceName),
				),
			},
			{
				RefreshState: true,
			},
		},
	})
}

func TestAccDatafyEC2EBSVolumeAttachment_replacedBy(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	resourceName := "aws_volume_attachment.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	replacedBy := ""

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeAttachmentDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeAttachmentConfig_basic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeAttachmentExists(ctx, resourceName),
					testAccCheckVolumeExists(ctx, "aws_ebs_volume.test", &v),
				),
			},
			{
				PreConfig: createDatafyReplacedByVolumeAttachment(ctx, &v, &replacedBy),
				Config:    testAccEBSVolumeAttachmentConfig_basic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeAttachmentExists(ctx, resourceName),
					resource.TestCheckResourceAttrPtr(resourceName, "volume_id", &replacedBy),
				),
			},
			{
				RefreshState: true,
			},
		},
	})
}

func TestAccDatafyEC2EBSVolumeAttachment_detach(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	resourceName := "aws_volume_attachment.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeAttachmentDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeAttachmentConfig_basic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeAttachmentExists(ctx, resourceName),
					testAccCheckVolumeExists(ctx, "aws_ebs_volume.test", &v),
				),
			},
			{
				PreConfig: createDatafyVolumeAttachment(ctx, &v, 10),
				Config:    testAccEBSVolumeAttachmentConfig_base(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeAttachmentDestroy(ctx),
				),
			},
			{
				RefreshState: true,
			},
		},
	})
}

func TestAccDatafyEC2EBSVolumeAttachment_attach(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	resourceName := "aws_volume_attachment.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeAttachmentDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeAttachmentConfig_base(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeAttachmentDestroy(ctx),
					testAccCheckVolumeExists(ctx, "aws_ebs_volume.test", &v),
				),
			},
			{
				PreConfig: createDatafyVolume(ctx, &v),
				Config:    testAccEBSVolumeAttachmentConfig_basic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeAttachmentExists(ctx, resourceName),
				),
			},
		},
	})
}

func testAccDatafyCheckVolumeAttachmentDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Conn()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "aws_volume_attachment" {
				continue
			}

			if _, err := tfec2.FindEBSVolumeAttachment(ctx, conn, rs.Primary.Attributes["volume_id"], rs.Primary.Attributes["instance_id"], rs.Primary.Attributes["device_name"]); err == nil {
				return fmt.Errorf("EBS Volume Attachment %s still exists", rs.Primary.ID)
			} else if !tfresource.NotFound(err) {
				return err
			}

			dvo, err := conn.DescribeVolumes(datafy.DescribeDatafiedVolumesInput(rs.Primary.Attributes["volume_id"]))
			if err != nil {
				return err
			}
			for _, volume := range dvo.Volumes {
				if len(volume.Attachments) != 0 {
					return fmt.Errorf("datafy EBS Volume Attachment still exists for source volume %s", rs.Primary.Attributes["volume_id"])
				}
			}
		}

		return nil
	}
}

func testAccDatafyCheckVolumeAttachmentExists(ctx context.Context, n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No EBS Volume ID is set")
		}

		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Conn()
		dvo, err := conn.DescribeVolumes(datafy.DescribeDatafiedVolumesInput(rs.Primary.Attributes["volume_id"]))
		if err != nil {
			return err
		}

		if len(dvo.Volumes) == 0 {
			return fmt.Errorf("no datafy volumes found for source volume %s", rs.Primary.Attributes["volume_id"])
		}

		for _, volume := range dvo.Volumes {
			if !slices.Any(volume.Attachments, func(va *ec2.VolumeAttachment) bool {
				return aws.StringValue(va.InstanceId) == rs.Primary.Attributes["instance_id"]
			}) {
				return fmt.Errorf("volume attachment not found for volume %s and instance %s", aws.StringValue(volume.VolumeId), rs.Primary.Attributes["instance_id"])
			}
		}

		return nil
	}
}

func createDatafyReplacedByVolumeAttachment(ctx context.Context, v *ec2.Volume, replacedBy *string) func() {
	return func() {
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Conn()

			oldVolumeId := aws.StringValue(v.VolumeId)
			dvo, err := conn.DetachVolume(&ec2.DetachVolumeInput{
				VolumeId: v.VolumeId,
				Force:    aws.Bool(true),
			})
			if err != nil {
				return err
			}
			if err := awsec2.NewVolumeAvailableWaiter(acctest.Provider.Meta().(*conns.AWSClient).EC2Client()).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{oldVolumeId},
			}, time.Minute); err != nil {
				return err
			}

			if _, err := conn.DeleteVolume(&ec2.DeleteVolumeInput{
				VolumeId: aws.String(oldVolumeId),
			}); err != nil {
				return err
			}
			if err := awsec2.NewVolumeDeletedWaiter(acctest.Provider.Meta().(*conns.AWSClient).EC2Client()).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{oldVolumeId},
			}, time.Minute); err != nil {
				return err
			}

			cvo, err := conn.CreateVolume(&ec2.CreateVolumeInput{
				AvailabilityZone: v.AvailabilityZone,
				Size:             v.Size,
				VolumeType:       v.VolumeType,
			})
			if err != nil {
				return err
			}
			if err := awsec2.NewVolumeAvailableWaiter(acctest.Provider.Meta().(*conns.AWSClient).EC2Client()).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{aws.StringValue(cvo.VolumeId)},
			}, time.Minute); err != nil {
				return err
			}

			if _, err := conn.AttachVolume(&ec2.AttachVolumeInput{
				Device:     dvo.Device,
				InstanceId: dvo.InstanceId,
				VolumeId:   cvo.VolumeId,
			}); err != nil {
				return err
			}

			if err := awsec2.NewVolumeInUseWaiter(acctest.Provider.Meta().(*conns.AWSClient).EC2Client()).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{aws.StringValue(cvo.VolumeId)},
			}, time.Minute); err != nil {
				return err
			}

			output, err := tfec2.FindEBSVolumeByID(ctx, conn, aws.StringValue(cvo.VolumeId))
			if err != nil {
				return err
			}

			*v = *output
			*replacedBy = *v.VolumeId

			acctest.DatafyClient.SetVolume(oldVolumeId, &datafy.Volume{
				Volume: &types.Volume{
					Size:       aws.Int32(int32(aws.Int64Value(v.Size))),
					Iops:       aws.Int32(int32(aws.Int64Value(v.Iops))),
					Throughput: aws.Int32(int32(aws.Int64Value(v.Throughput))),
					VolumeType: types.VolumeType(aws.StringValue(v.VolumeType)),
					Tags:       slices.ApplyToAll(v.Tags, func(t *ec2.Tag) types.Tag { return types.Tag{Key: t.Key, Value: t.Value} }),
				},
				HasSource:  false,
				IsManaged:  false,
				IsDatafied: false,
				ReplacedBy: aws.StringValue(cvo.VolumeId),
			})
			return nil
		}()
		if err != nil {
			panic(err)
		}
	}
}

func createDatafyVolumeAttachment(ctx context.Context, v *ec2.Volume, size int) func() {
	return func() {
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Conn()

			dvo, err := conn.DetachVolume(&ec2.DetachVolumeInput{
				VolumeId: v.VolumeId,
				Force:    aws.Bool(true),
			})
			if err != nil {
				return err
			}

			if err := awsec2.NewVolumeAvailableWaiter(acctest.Provider.Meta().(*conns.AWSClient).EC2Client()).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{aws.StringValue(v.VolumeId)},
			}, time.Minute); err != nil {
				return err
			}

			if _, err := conn.DeleteVolume(&ec2.DeleteVolumeInput{VolumeId: v.VolumeId}); err != nil {
				return err
			}
			if err := awsec2.NewVolumeDeletedWaiter(acctest.Provider.Meta().(*conns.AWSClient).EC2Client()).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{aws.StringValue(v.VolumeId)},
			}, time.Minute); err != nil {
				return err
			}

			for i := 0; i < 2; i++ {
				if _, err := conn.CreateVolume(&ec2.CreateVolumeInput{
					AvailabilityZone: v.AvailabilityZone,
					Size:             aws.Int64(int64(size)),
					VolumeType:       v.VolumeType,
					TagSpecifications: []*ec2.TagSpecification{
						{
							ResourceType: aws.String("volume"),
							Tags: []*ec2.Tag{
								{
									Key:   aws.String("Managed-By"),
									Value: aws.String("Datafy.io"),
								},
								{
									Key:   aws.String("datafy:source-volume:id"),
									Value: v.VolumeId,
								},
							},
						},
					},
				}); err != nil {
					return err
				}
			}

			acctest.DatafyClient.AttachVolume(aws.StringValue(dvo.InstanceId), aws.StringValue(v.VolumeId), "")
			acctest.DatafyClient.SetVolume(aws.StringValue(v.VolumeId), &datafy.Volume{
				Volume: &types.Volume{
					Size:       aws.Int32(int32(size)),
					Iops:       aws.Int32(int32(aws.Int64Value(v.Iops))),
					Throughput: aws.Int32(int32(aws.Int64Value(v.Throughput))),
					VolumeType: types.VolumeType(aws.StringValue(v.VolumeType)),
					Tags:       slices.ApplyToAll(v.Tags, func(t *ec2.Tag) types.Tag { return types.Tag{Key: t.Key, Value: t.Value} }),
				},
				HasSource:  false,
				IsManaged:  true,
				IsDatafied: true,
				ReplacedBy: "",
			})
			return nil
		}()
		if err != nil {
			panic(err)
		}
	}
}
