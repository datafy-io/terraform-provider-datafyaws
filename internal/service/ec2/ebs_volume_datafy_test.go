// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ec2_test

import (
	"context"
	"fmt"
	"regexp"
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
	"github.com/hashicorp/terraform-provider-aws/names"
)

func TestAccDatafyEC2EBSVolume_basic(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	var dv []*ec2.Volume
	resourceName := "aws_ebs_volume.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, resourceName, &v),
				),
			},
			{
				PreConfig: createDatafyVolume(ctx, &v),
				Config:    testAccEBSVolumeConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
				),
			},
			{
				RefreshState: true,
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_replacedBy(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	resourceName := "aws_ebs_volume.test"
	replacedBy := ""

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, resourceName, &v),
				),
			},
			{
				PreConfig: createDatafyReplacedByVolume(ctx, &v, &replacedBy),
				Config:    testAccEBSVolumeConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, resourceName, &v),
					resource.TestCheckResourceAttrPtr(resourceName, names.AttrID, &replacedBy),
				),
			},
			{
				RefreshState: true,
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_blockModifyType(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	var dv []*ec2.Volume
	resourceName := "aws_ebs_volume.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_tags1("Name", rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, resourceName, &v),
				),
			},
			{
				PreConfig: createDatafyVolume(ctx, &v),
				Config:    testAccEBSVolumeConfig_updateType(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
				),
				ExpectError: regexp.MustCompile(`can't modify datafied EBS Volume .*`),
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_modifyOnlyTags(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	var dv []*ec2.Volume
	resourceName := "aws_ebs_volume.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_tags1("Name", sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, resourceName, &v),
				),
			},
			{
				PreConfig: createDatafyVolume(ctx, &v),
				Config:    testAccEBSVolumeConfig_tags1("Name", rName),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					testAccDatafyCheckTagExists(ctx, &dv, "Name", rName),
				),
			},
			{
				RefreshState: true,
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_modifyOnlySize(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	var dv []*ec2.Volume
	resourceName := "aws_ebs_volume.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_updateSize(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, resourceName, &v),
				),
			},
			{
				PreConfig: createDatafyVolume(ctx, &v),
				Config:    testAccEBSVolumeConfig_sizeTypeIOPSThroughput(rName, "8", "gp2", "3000", "125"),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					testAccDatafyCheckTagExists(ctx, &dv, "Name", rName),
				),
			},
			{
				RefreshState: true,
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_restoreFromSnapshot(t *testing.T) {
	ctx := acctest.Context(t)
	var dv []*ec2.Volume
	resourceName := "aws_ebs_volume.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	dsnapId := sdkacctest.RandomWithPrefix("dsnap")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				PreConfig: createDatafyVolumeSnapshot(ctx, dsnapId, 10),
				Config:    testAccDatafyEBSVolumeConfig_dsnap(rName, dsnapId),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					testAccDatafyCheckTagExists(ctx, &dv, "Name", rName),
				),
			},
			{
				RefreshState: true,
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_restoredFromSnapshotTag(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	resourceName := "aws_ebs_volume.test"
	snapId := sdkacctest.RandomWithPrefix("dsnap")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, resourceName, &v),
				),
			},
			{
				PreConfig:    addRestoredFromSnapshotTag(ctx, &v, snapId),
				RefreshState: true,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "snapshot_id", snapId),
				),
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_rejectDatafyTagOnCreate(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config:      testAccDatafyEBSVolumeConfig_withDatafyTag(rName),
				ExpectError: regexp.MustCompile(`tag key "datafy:.*" uses the reserved "datafy:" prefix`),
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_rejectDatafyTagOnUpdate(t *testing.T) {
	ctx := acctest.Context(t)
	var v ec2.Volume
	resourceName := "aws_ebs_volume.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_tags1("Name", rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, resourceName, &v),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig_withDatafyTag(rName),
				ExpectError: regexp.MustCompile(`tag key "datafy:.*" uses the reserved "datafy:" prefix`),
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_rejectSnapshotInGroup(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config:      testAccDatafyEBSVolumeConfig_snapshotInGroup(rName),
				ExpectError: regexp.MustCompile(`cannot create EBS Volume from snapshot .* this snapshot belongs to Datafy snapshot`),
			},
		},
	})
}

func addRestoredFromSnapshotTag(ctx context.Context, v *ec2.Volume, snapId string) func() {
	return func() {
		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client()
		if _, err := conn.CreateTags(ctx, &awsec2.CreateTagsInput{
			Resources: []string{aws.StringValue(v.VolumeId)},
			Tags: []types.Tag{
				{
					Key:   aws.String("datafy:restored-from-snapshot:id"),
					Value: aws.String(snapId),
				},
			},
		}); err != nil {
			panic(err)
		}
	}
}

func createDatafyVolume(ctx context.Context, v *ec2.Volume) func() {
	return func() {
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Conn()

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
					Size:             aws.Int64(1),
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

			acctest.DatafyClient.SetVolume(aws.StringValue(v.VolumeId), &datafy.Volume{
				Volume: &types.Volume{
					Size:       aws.Int32(int32(aws.Int64Value(v.Size))),
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

func createDatafyVolumeSnapshot(ctx context.Context, dsnapId string, size int) func() {
	return func() {
		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Conn()

		o, err := conn.DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{})
		if err != nil {
			panic(err)
		}

		volume, err := conn.CreateVolume(&ec2.CreateVolumeInput{
			AvailabilityZone: o.AvailabilityZones[0].ZoneName,
			Size:             aws.Int64(1),
			VolumeType:       aws.String("gp2"),
			Encrypted:        aws.Bool(true),
			KmsKeyId:         aws.String("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		})
		if err != nil {
			panic(err)
		}

		acctest.DatafyClient.SetRestoredVolume(dsnapId, &datafy.RestoredVolume{
			VolumeId:     aws.StringValue(volume.VolumeId),
			VolumeSizeGB: int32(size),
		})
	}
}

func createDatafyReplacedByVolume(ctx context.Context, v *ec2.Volume, replacedBy *string) func() {
	return func() {
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Conn()

			oldVolumeId := aws.StringValue(v.VolumeId)
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

func testAccDatafyCheckVolumeExists(ctx context.Context, n string, dv *[]*ec2.Volume) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No EBS Volume ID is set")
		}

		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Conn()
		dvo, err := conn.DescribeVolumes(datafy.DescribeDatafiedVolumesInput(rs.Primary.ID))
		if err != nil {
			return err
		}

		if len(dvo.Volumes) == 0 {
			return fmt.Errorf("no datafy volumes found for source volume %s", rs.Primary.ID)
		}

		*dv = dvo.Volumes
		return nil
	}
}

func testAccDatafyCheckTagExists(ctx context.Context, dv *[]*ec2.Volume, key, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, v := range *dv {
			if !slices.Any(v.Tags, func(t *ec2.Tag) bool {
				return aws.StringValue(t.Key) == key && aws.StringValue(t.Value) == value
			}) {
				return fmt.Errorf("tag %s=%s not found on volume %s", key, value, aws.StringValue(v.VolumeId))
			}
		}
		return nil
	}
}

func testAccDatafyCheckVolumeDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Conn()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "aws_ebs_volume" {
				continue
			}

			if volume, _ := acctest.DatafyClient.GetVolume(rs.Primary.ID); volume != nil && volume.IsDatafied {
				dvo, err := conn.DescribeVolumes(datafy.DescribeDatafiedVolumesInput(rs.Primary.ID))
				if err != nil {
					return err
				}
				if len(dvo.Volumes) != 0 {
					return fmt.Errorf("datafy EBS Volumes still exists for source volume %s", rs.Primary.ID)
				}
			}
		}

		return nil
	}
}

func testAccDatafyEBSVolumeConfig_dsnap(rName string, dsnapId string) string {
	return acctest.ConfigCompose(
		acctest.ConfigAvailableAZsNoOptIn(),
		fmt.Sprintf(`
resource "aws_ebs_volume" "test" {
  availability_zone = data.aws_availability_zones.available.names[0]
  snapshot_id       = %[2]q

  tags = {
    Name = %[1]q
  }
}
`, rName, dsnapId))
}

func testAccDatafyEBSVolumeConfig_withDatafyTag(rName string) string {
	return acctest.ConfigCompose(
		acctest.ConfigAvailableAZsNoOptIn(),
		fmt.Sprintf(`
resource "aws_ebs_volume" "test" {
  availability_zone = data.aws_availability_zones.available.names[0]
  size              = 1

  tags = {
    Name         = %[1]q
    "datafy:foo" = "bar"
  }
}
`, rName))
}

func testAccDatafyEBSVolumeConfig_snapshotInGroup(rName string) string {
	return acctest.ConfigCompose(
		acctest.ConfigAvailableAZsNoOptIn(),
		fmt.Sprintf(`
resource "aws_ebs_volume" "source" {
  availability_zone = data.aws_availability_zones.available.names[0]
  size              = 1
}

resource "aws_ebs_snapshot" "test" {
  volume_id = aws_ebs_volume.source.id

  tags = {
    "datafy:snapshot:id" = "dsnap-test-group"
  }
}

resource "aws_ebs_volume" "test" {
  availability_zone = data.aws_availability_zones.available.names[0]
  snapshot_id       = aws_ebs_snapshot.test.id

  tags = {
    Name = %[1]q
  }
}
`, rName))
}
