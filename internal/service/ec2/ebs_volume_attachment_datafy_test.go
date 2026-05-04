// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ec2_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awstypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	sdkacctest "github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-provider-aws/internal/acctest"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
	tfec2 "github.com/hashicorp/terraform-provider-aws/internal/service/ec2"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/names"
)

func TestAccDatafyEC2EBSVolumeAttachment_basic(t *testing.T) {
	ctx := acctest.Context(t)
	var v awstypes.Volume
	resourceName := "aws_volume_attachment.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeAttachmentDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeAttachmentConfig_basic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEBSVolumeAttachmentExists(ctx, t, resourceName),
					testAccCheckVolumeExists(ctx, t, "aws_ebs_volume.test", &v),
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
	var v awstypes.Volume
	resourceName := "aws_volume_attachment.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	replacedBy := ""

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeAttachmentDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeAttachmentConfig_basic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEBSVolumeAttachmentExists(ctx, t, resourceName),
					testAccCheckVolumeExists(ctx, t, "aws_ebs_volume.test", &v),
				),
			},
			{
				PreConfig: createDatafyReplacedByVolumeAttachment(ctx, &v, &replacedBy),
				Config:    testAccEBSVolumeAttachmentConfig_basic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEBSVolumeAttachmentExists(ctx, t, resourceName),
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
	var v awstypes.Volume
	resourceName := "aws_volume_attachment.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeAttachmentDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeAttachmentConfig_basic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEBSVolumeAttachmentExists(ctx, t, resourceName),
					testAccCheckVolumeExists(ctx, t, "aws_ebs_volume.test", &v),
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
	var v awstypes.Volume
	resourceName := "aws_volume_attachment.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeAttachmentDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeAttachmentConfig_base(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEBSVolumeAttachmentDestroy(ctx, t),
					testAccCheckVolumeExists(ctx, t, "aws_ebs_volume.test", &v),
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
		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "aws_volume_attachment" {
				continue
			}

			if _, err := tfec2.FindEBSVolumeAttachment(ctx, conn, rs.Primary.Attributes["volume_id"], rs.Primary.Attributes[names.AttrInstanceID], rs.Primary.Attributes[names.AttrDeviceName]); err == nil {
				return fmt.Errorf("EBS Volume Attachment %s still exists", rs.Primary.ID)
			} else if !tfresource.NotFound(err) {
				return err
			}

			dvo, err := conn.DescribeVolumes(ctx, datafy.DescribeDatafiedVolumesInput(rs.Primary.Attributes["volume_id"]))
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

		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)
		dvo, err := conn.DescribeVolumes(ctx, datafy.DescribeDatafiedVolumesInput(rs.Primary.Attributes["volume_id"]))
		if err != nil {
			return err
		}

		if len(dvo.Volumes) == 0 {
			return fmt.Errorf("no datafy volumes found for source volume %s", rs.Primary.Attributes["volume_id"])
		}

		for _, volume := range dvo.Volumes {
			if !slices.ContainsFunc(volume.Attachments, func(va awstypes.VolumeAttachment) bool {
				return aws.ToString(va.InstanceId) == rs.Primary.Attributes[names.AttrInstanceID]
			}) {
				return fmt.Errorf("volume attachment not found for volume %s and instance %s", aws.ToString(volume.VolumeId), rs.Primary.Attributes[names.AttrInstanceID])
			}
		}

		return nil
	}
}

func createDatafyReplacedByVolumeAttachment(ctx context.Context, v *awstypes.Volume, replacedBy *string) func() {
	return func() {
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)

			oldVolumeId := aws.ToString(v.VolumeId)
			dvo, err := conn.DetachVolume(ctx, &awsec2.DetachVolumeInput{
				VolumeId: v.VolumeId,
				Force:    aws.Bool(true),
			})
			if err != nil {
				return err
			}
			if err := awsec2.NewVolumeAvailableWaiter(conn).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{oldVolumeId},
			}, time.Minute); err != nil {
				return err
			}

			if _, err := conn.DeleteVolume(ctx, &awsec2.DeleteVolumeInput{
				VolumeId: aws.String(oldVolumeId),
			}); err != nil {
				return err
			}
			if err := awsec2.NewVolumeDeletedWaiter(conn).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{oldVolumeId},
			}, time.Minute); err != nil {
				return err
			}

			cvo, err := conn.CreateVolume(ctx, &awsec2.CreateVolumeInput{
				AvailabilityZone: v.AvailabilityZone,
				Size:             v.Size,
				VolumeType:       v.VolumeType,
			})
			if err != nil {
				return err
			}
			if err := awsec2.NewVolumeAvailableWaiter(conn).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{aws.ToString(cvo.VolumeId)},
			}, time.Minute); err != nil {
				return err
			}

			if _, err := conn.AttachVolume(ctx, &awsec2.AttachVolumeInput{
				Device:     dvo.Device,
				InstanceId: dvo.InstanceId,
				VolumeId:   cvo.VolumeId,
			}); err != nil {
				return err
			}

			if err := awsec2.NewVolumeInUseWaiter(conn).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{aws.ToString(cvo.VolumeId)},
			}, time.Minute); err != nil {
				return err
			}

			output, err := tfec2.FindEBSVolumeByID(ctx, conn, aws.ToString(cvo.VolumeId))
			if err != nil {
				return err
			}

			*v = *output
			*replacedBy = *v.VolumeId

			acctest.DatafyClient.SetVolume(oldVolumeId, &datafy.Volume{
				Volume:     v,
				HasSource:  false,
				IsManaged:  false,
				IsDatafied: false,
				ReplacedBy: aws.ToString(cvo.VolumeId),
			})
			return nil
		}()
		if err != nil {
			panic(err)
		}
	}
}

func createDatafyVolumeAttachment(ctx context.Context, v *awstypes.Volume, size int) func() {
	return func() {
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)

			dvo, err := conn.DetachVolume(ctx, &awsec2.DetachVolumeInput{
				VolumeId: v.VolumeId,
				Force:    aws.Bool(true),
			})
			if err != nil {
				return err
			}

			if err := awsec2.NewVolumeAvailableWaiter(conn).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{aws.ToString(v.VolumeId)},
			}, time.Minute); err != nil {
				return err
			}

			if _, err := conn.DeleteVolume(ctx, &awsec2.DeleteVolumeInput{VolumeId: v.VolumeId}); err != nil {
				return err
			}
			if err := awsec2.NewVolumeDeletedWaiter(conn).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{aws.ToString(v.VolumeId)},
			}, time.Minute); err != nil {
				return err
			}

			for range 2 {
				if _, err := conn.CreateVolume(ctx, &awsec2.CreateVolumeInput{
					AvailabilityZone: v.AvailabilityZone,
					Size:             aws.Int32(int32(size)),
					VolumeType:       v.VolumeType,
					TagSpecifications: []awstypes.TagSpecification{
						{
							ResourceType: awstypes.ResourceTypeVolume,
							Tags: []awstypes.Tag{
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

			acctest.DatafyClient.AttachVolume(aws.ToString(dvo.InstanceId), aws.ToString(v.VolumeId), "")
			acctest.DatafyClient.SetVolume(aws.ToString(v.VolumeId), &datafy.Volume{
				Volume:     v,
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
