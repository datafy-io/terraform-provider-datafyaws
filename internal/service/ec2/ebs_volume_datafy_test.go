// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ec2_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	var dv datafyVolume
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
	var dv datafyVolume
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
	var dv datafyVolume
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
	var dv datafyVolume
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
	var dv datafyVolume
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

// undatafyNativeVolume simulates undatafying a native volume from the datafy
// UI: a real standard volume replaces it, the datafy shard volumes are removed,
// and the mock reports the old (synthetic) volume id as replaced and unmanaged.
func undatafyNativeVolume(ctx context.Context, dv *datafyVolume, rName string) func() {
	return func() {
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Conn()

			cvo, err := conn.CreateVolume(&ec2.CreateVolumeInput{
				AvailabilityZone: dv.volumes[0].AvailabilityZone,
				Size:             aws.Int64(1),
				TagSpecifications: []*ec2.TagSpecification{
					{
						ResourceType: aws.String(ec2.ResourceTypeVolume),
						Tags: []*ec2.Tag{
							{
								Key:   aws.String("Name"),
								Value: aws.String(rName),
							},
						},
					},
				},
			})
			if err != nil {
				return err
			}

			replacement, err := tfec2.FindEBSVolumeByID(ctx, conn, aws.StringValue(cvo.VolumeId))
			if err != nil {
				return err
			}

			for _, shard := range dv.volumes {
				if _, err := conn.DeleteVolume(&ec2.DeleteVolumeInput{VolumeId: shard.VolumeId}); err != nil {
					return err
				}
			}

			acctest.DatafyClient.SetVolume(dv.volumeId, &datafy.Volume{
				Volume: &types.Volume{
					Size:       aws.Int32(int32(aws.Int64Value(replacement.Size))),
					Iops:       aws.Int32(int32(aws.Int64Value(replacement.Iops))),
					Throughput: aws.Int32(int32(aws.Int64Value(replacement.Throughput))),
					VolumeType: types.VolumeType(aws.StringValue(replacement.VolumeType)),
					Tags:       slices.ApplyToAll(replacement.Tags, func(t *ec2.Tag) types.Tag { return types.Tag{Key: t.Key, Value: t.Value} }),
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

// datafyVolume captures the datafy view of a managed EBS volume: the id of
// the managed (source) volume as stored in state, and the real datafy volumes
// backing it.
type datafyVolume struct {
	volumeId string
	volumes  []*ec2.Volume
}

func testAccDatafyCheckVolumeExists(ctx context.Context, n string, dv *datafyVolume) resource.TestCheckFunc {
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

		dv.volumeId = rs.Primary.ID
		dv.volumes = dvo.Volumes
		return nil
	}
}

func testAccDatafyCheckTagExists(ctx context.Context, dv *datafyVolume, key, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, v := range dv.volumes {
			if !slices.Any(v.Tags, func(t *ec2.Tag) bool {
				return aws.StringValue(t.Key) == key && aws.StringValue(t.Value) == value
			}) {
				return fmt.Errorf("tag %s=%s not found on volume %s", key, value, aws.StringValue(v.VolumeId))
			}
		}
		return nil
	}
}

// testAccDatafyCheckArraySize asserts how many backing volumes the array was created with.
func testAccDatafyCheckArraySize(dv *datafyVolume, want int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if len(dv.volumes) != want {
			return fmt.Errorf("expected %d datafy volumes for source volume %s, got %d", want, dv.volumeId, len(dv.volumes))
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

func TestAccDatafyEC2EBSVolume_createAutoscaling(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true)),
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

// A volume asking for performance: the array is created datafied and its tier is what sizes
// it. The replan step is the regression guard — the legacy SDK stores
// the zero value for an attribute the configuration never held, and reading that back as an
// offboarding attempt failed every plan after the first.
func TestAccDatafyEC2EBSVolume_createPerformanceArray(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig(rName, withPerformance(true), withPerformanceTier(4, 12000, 500)),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					testAccDatafyCheckArraySize(&dv, 4),
					testAccDatafyCheckTagExists(ctx, &dv, "Name", rName),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrPerformanceTier, "4"),
					// What the array delivers, not the share one member was provisioned at.
					resource.TestCheckResourceAttr(resourceName, "iops", "12000"),
					resource.TestCheckResourceAttr(resourceName, "throughput", "500"),
				),
			},
			{
				Config:   testAccDatafyEBSVolumeConfig(rName, withPerformance(true), withPerformanceTier(4, 12000, 500)),
				PlanOnly: true,
			},
		},
	})
}

// Both optimizations at once.
func TestAccDatafyEC2EBSVolume_createAutoscalingPerformance(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withPerformance(true), withPerformanceTier(6, 18000, 750)),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					testAccDatafyCheckArraySize(&dv, 6),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrAutoscaling, "true"),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrPerformance, "true"),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrPerformanceTier, "6"),
				),
			},
			{
				Config:   testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withPerformance(true), withPerformanceTier(6, 18000, 750)),
				PlanOnly: true,
			},
		},
	})
}

// The tier and the performance flag only mean anything together: the array IS the performance
// optimization. Datafy sizes it and resolves what the volume's performance means at that size,
// so only the pairing is checked here.
func TestAccDatafyEC2EBSVolume_rejectPerformanceTierPairing(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withPerformanceTier(4, 12000, 500)),
				ExpectError: regexp.MustCompile("`datafy_performance_tier` is only valid when `datafy_performance` is true"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withPerformance(true)),
				ExpectError: regexp.MustCompile("`datafy_performance_tier` must be set when `datafy_performance` is true"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withPerformance(true)),
				ExpectError: regexp.MustCompile("`datafy_performance_tier` must be set when `datafy_performance` is true"),
			},
		},
	})
}

// Datafy owns the volume type of its volumes (always gp3).
func TestAccDatafyEC2EBSVolume_rejectTypeOnDatafyVolume(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withVolumeType("gp2")),
				ExpectError: regexp.MustCompile("`type` must not be set when `datafy_autoscaling` or `datafy_performance` is true"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withVolumeType("gp3")),
				ExpectError: regexp.MustCompile("`type` must not be set when `datafy_autoscaling` or `datafy_performance` is true"),
			},
		},
	})
}

// on a native (datafied) volume, changing a datafy flag errors, and removing one from the
// configuration errors as long as the volume is datafied.
func TestAccDatafyEC2EBSVolume_datafyFlagsDatafiedNative(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true)),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withPerformance(true), withPerformanceTier(4, 12000, 500)),
				ExpectError: regexp.MustCompile("changing `datafy_performance` of an existing EBS Volume"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName),
				ExpectError: regexp.MustCompile("removing `datafy_autoscaling` from EBS Volume .* is not allowed while the volume is datafied"),
			},
		},
	})
}

// a native volume that was undatafied from the UI (replaced by a standard volume).
// Changing a datafy flag still errors, but removing one from the configuration is allowed - it
// applies in-place, leaving false in state (offboarding; the legacy Plugin SDK writes the zero
// value for removed optional primitives, not null).
func TestAccDatafyEC2EBSVolume_datafyFlagsUndatafiedFromUI(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true)),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrAutoscaling, "true"),
				),
			},
			{
				PreConfig:   undatafyNativeVolume(ctx, &dv, rName),
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withPerformance(true), withPerformanceTier(4, 12000, 500)),
				ExpectError: regexp.MustCompile("changing `datafy_performance` of an existing EBS Volume"),
			},
			{
				Config: testAccDatafyEBSVolumeConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, datafy.AttrAutoscaling, "false"),
				),
			},
			{
				Config:   testAccDatafyEBSVolumeConfig(rName),
				PlanOnly: true,
			},
		},
	})
}

// a standard volume that was datafied from the UI. Asking for an optimization errors: the
// volume already exists, and datafy owns it.
func TestAccDatafyEC2EBSVolume_datafyFlagsDatafied(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var v ec2.Volume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, resourceName, &v),
				),
			},
			{
				PreConfig:   createDatafyVolume(ctx, &v),
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true)),
				ExpectError: regexp.MustCompile("changing `datafy_autoscaling` of an existing EBS Volume"),
			},
		},
	})
}

// A volume created without the attributes holds nil for them in state. Adding either
// afterwards errors; keeping them omitted stays a noop.
func TestAccDatafyEC2EBSVolume_datafyFlagsAddToExisting(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var v ec2.Volume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, resourceName, &v),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true)),
				ExpectError: regexp.MustCompile("changing `datafy_autoscaling` of an existing EBS Volume"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withPerformance(true), withPerformanceTier(4, 12000, 500)),
				ExpectError: regexp.MustCompile("changing `datafy_performance` of an existing EBS Volume"),
			},
			{
				Config:   testAccDatafyEBSVolumeConfig(rName),
				PlanOnly: true,
			},
		},
	})
}

// Both attributes are decided when the volume is created. Changing either is refused, and so
// is dropping one while the volume is datafied.
func TestAccDatafyEC2EBSVolume_performanceTierImmutable(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, ec2.EndpointsID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withPerformance(true), withPerformanceTier(4, 12000, 500)),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrPerformanceTier, "4"),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withPerformance(true), withPerformanceTier(8, 24000, 1000)),
				ExpectError: regexp.MustCompile("changing `datafy_performance_tier` of an existing EBS Volume"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(false), withPerformance(true), withPerformanceTier(4, 12000, 500)),
				ExpectError: regexp.MustCompile("changing `datafy_autoscaling` of an existing EBS Volume"),
			},
			{
				// Dropping the tier alone is still dropping it. A datafied volume refuses the
				// removal outright, before the flag and the tier are weighed against each other.
				Config:      testAccDatafyEBSVolumeConfig(rName, withAutoscaling(true), withPerformance(true)),
				ExpectError: regexp.MustCompile("removing `datafy_performance_tier` from EBS Volume .* is not allowed while the volume is datafied"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName),
				ExpectError: regexp.MustCompile("removing `datafy_(autoscaling|performance|performance_tier)` from EBS Volume .* is not allowed while the volume is datafied"),
			},
		},
	})
}

func testAccDatafyEBSVolumeConfig(rName string, attrs ...string) string {
	var body string
	for _, attr := range attrs {
		body += attr + "\n"
	}

	return acctest.ConfigCompose(
		acctest.ConfigAvailableAZsNoOptIn(),
		fmt.Sprintf(`
resource "aws_ebs_volume" "test" {
  availability_zone             = data.aws_availability_zones.available.names[0]
  size                          = 100
%[2]s
  tags = {
    Name = %[1]q
  }
}
`, rName, body))
}

// attr renders one of those attributes, aligned on the longest name the tests use.
func attr(name, value string) string {
	return fmt.Sprintf("  %-*s = %s", len(datafy.AttrPerformanceTier), name, value)
}

func withVolumeType(volumeType string) string {
	return attr("type", strconv.Quote(volumeType))
}

func withAutoscaling(autoscaling bool) string {
	return attr(datafy.AttrAutoscaling, strconv.FormatBool(autoscaling))
}

func withPerformance(performance bool) string {
	return attr(datafy.AttrPerformance, strconv.FormatBool(performance))
}

// withPerformanceTier sizes the array and states the performance it has to deliver. `iops` and
// `throughput` are ARRAY TOTALS, so they scale with the member count rather than being
// per-volume numbers.
func withPerformanceTier(tier, iops, throughput int) string {
	return strings.Join([]string{
		attr(datafy.AttrPerformanceTier, strconv.Itoa(tier)),
		attr("iops", strconv.Itoa(iops)),
		attr("throughput", strconv.Itoa(throughput)),
	}, "\n")
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
