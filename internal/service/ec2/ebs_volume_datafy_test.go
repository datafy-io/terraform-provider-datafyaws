// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ec2_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/YakDriver/regexache"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awstypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	sdkacctest "github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-provider-aws/internal/acctest"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
	tfec2 "github.com/hashicorp/terraform-provider-aws/internal/service/ec2"
	"github.com/hashicorp/terraform-provider-aws/names"
)

func TestAccDatafyEC2EBSVolume_basic(t *testing.T) {
	ctx := acctest.Context(t)
	resourceName := "aws_ebs_volume.test"

	var v awstypes.Volume
	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
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
	resourceName := "aws_ebs_volume.test"
	replacedBy := ""

	var v awstypes.Volume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx, t),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
				),
			},
			{
				PreConfig: createDatafyReplacedByVolume(ctx, &v, &replacedBy),
				Config:    testAccEBSVolumeConfig_basic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
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
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"
	var v awstypes.Volume
	var dv datafyVolume

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_tags1("Name", rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
				),
			},
			{
				PreConfig: createDatafyVolume(ctx, &v),
				Config:    testAccEBSVolumeConfig_updateType(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
				),
				ExpectError: regexache.MustCompile(`can't modify datafied EBS Volume .*`),
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_modifyOnlyTags(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var v awstypes.Volume
	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_tags1("Name", sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
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
	resourceName := "aws_ebs_volume.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	var v awstypes.Volume
	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_updateSize(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
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
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	dsnapId := sdkacctest.RandomWithPrefix("dsnap")
	resourceName := "aws_ebs_volume.test"

	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
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

func TestAccDatafyEC2EBSVolume_rejectDatafyTagOnCreate(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx, t),
		Steps: []resource.TestStep{
			{
				Config:      testAccDatafyEBSVolumeConfig_withDatafyTag(rName),
				ExpectError: regexache.MustCompile(`tag key "datafy:.*" uses the reserved "datafy:" prefix`),
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_rejectDatafyTagOnUpdate(t *testing.T) {
	ctx := acctest.Context(t)
	resourceName := "aws_ebs_volume.test"
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	var v awstypes.Volume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx, t),
		Steps: []resource.TestStep{
			{
				Config: testAccEBSVolumeConfig_tags1("Name", rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig_withDatafyTag(rName),
				ExpectError: regexache.MustCompile(`tag key "datafy:.*" uses the reserved "datafy:" prefix`),
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_rejectSnapshotInGroup(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx, t),
		Steps: []resource.TestStep{
			{
				Config:      testAccDatafyEBSVolumeConfig_snapshotInGroup(rName),
				ExpectError: regexache.MustCompile(`cannot create EBS Volume from snapshot .* this snapshot belongs to Datafy snapshot`),
			},
		},
	})
}

func TestAccDatafyEC2EBSVolume_createNative(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig_autoscalingNative(rName, true),
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

// on a native (datafied) volume, flipping the flag to false errors, and removing it from the
// configuration errors as long as the volume is datafied.
func TestAccDatafyEC2EBSVolume_autoscalingNativeDatafiedNative(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig_autoscalingNative(rName, true),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig_autoscalingNative(rName, false),
				ExpectError: regexache.MustCompile("changing `autoscaling_native` of an existing EBS Volume"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig_autoscalingNativeRemoved(rName),
				ExpectError: regexache.MustCompile("removing `autoscaling_native` from EBS Volume .* is not allowed while the volume is datafied"),
			},
		},
	})
}

// a native volume that was undatafied from the UI (replaced by a standard volume).
// Flipping the flag to false still errors, but removing it from the configuration is
// allowed - it applies in-place, leaving false in state (offboarding; the legacy
// Plugin SDK writes the zero value for removed optional primitives, not null).
func TestAccDatafyEC2EBSVolume_autoscalingNativeUndatafiedFromUI(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var dv datafyVolume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx, t),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig_autoscalingNative(rName, true),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					resource.TestCheckResourceAttr(resourceName, "autoscaling_native", "true"),
				),
			},
			{
				PreConfig:   undatafyNativeVolume(ctx, &dv, rName),
				Config:      testAccDatafyEBSVolumeConfig_autoscalingNative(rName, false),
				ExpectError: regexache.MustCompile("changing `autoscaling_native` of an existing EBS Volume"),
			},
			{
				Config: testAccDatafyEBSVolumeConfig_autoscalingNativeRemoved(rName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "autoscaling_native", "false"),
				),
			},
			{
				Config: testAccDatafyEBSVolumeConfig_autoscalingNativeRemoved(rName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

// a standard volume (flag false) that was datafied from the UI.
// Flipping the flag to true errors, and removing it from the configuration
// errors as long as the volume is datafied.
func TestAccDatafyEC2EBSVolume_autoscalingNativeDatafied(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var v awstypes.Volume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig_autoscalingNative(rName, false),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
				),
			},
			{
				PreConfig:   createDatafyVolume(ctx, &v),
				Config:      testAccDatafyEBSVolumeConfig_autoscalingNative(rName, true),
				ExpectError: regexache.MustCompile("changing `autoscaling_native` of an existing EBS Volume"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig_autoscalingNativeRemoved(rName),
				ExpectError: regexache.MustCompile("removing `autoscaling_native` from EBS Volume .* is not allowed while the volume is datafied"),
			},
		},
	})
}

// a standard volume (flag false) that datafy does not manage.
// Flipping the flag to true errors, and removing it from the configuration is
// a noop — offboarding is allowed.
func TestAccDatafyEC2EBSVolume_autoscalingNativeUndatafied(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var v awstypes.Volume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx, t),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig_autoscalingNative(rName, false),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig_autoscalingNative(rName, true),
				ExpectError: regexache.MustCompile("changing `autoscaling_native` of an existing EBS Volume"),
			},
			{
				Config: testAccDatafyEBSVolumeConfig_autoscalingNativeRemoved(rName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

// A volume created without the flag holds nil for it in state. Adding the flag
// to the configuration afterwards (true or false) - errors; keeping it omitted stays a noop.
func TestAccDatafyEC2EBSVolume_autoscalingNativeAddToExisting(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)
	resourceName := "aws_ebs_volume.test"

	var v awstypes.Volume
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroy(ctx, t),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig_autoscalingNativeRemoved(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig_autoscalingNative(rName, false),
				ExpectError: regexache.MustCompile("changing `autoscaling_native` of an existing EBS Volume"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig_autoscalingNative(rName, true),
				ExpectError: regexache.MustCompile("changing `autoscaling_native` of an existing EBS Volume"),
			},
			{
				Config: testAccDatafyEBSVolumeConfig_autoscalingNativeRemoved(rName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

// Datafy owns the volume type of native volumes (always gp3), so any explicit
// `type` (even gp3) must be rejected at plan time; only unset is allowed.
func TestAccDatafyEC2EBSVolume_rejectAutoscalingNativeWithType(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config:      testAccDatafyEBSVolumeConfig_autoscalingNativeType(rName, "gp2"),
				ExpectError: regexache.MustCompile("`type` must not be set when autoscaling_native is true"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig_autoscalingNativeType(rName, "gp3"),
				ExpectError: regexache.MustCompile("`type` must not be set when autoscaling_native is true"),
			},
		},
	})
}

func createDatafyVolume(ctx context.Context, v *awstypes.Volume) func() {
	return func() {
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)

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
					Size:             aws.Int32(1),
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

func createDatafyVolumeSnapshot(ctx context.Context, dsnapId string, size int) func() {
	return func() {
		// AWS rejects synthetic vol-ids in DescribeVolumes with
		// InvalidParameterValue, not InvalidVolume.NotFound. The provider's
		// Read path only treats the latter as "missing → fall back to Datafy".
		// Harvest a real AWS-issued vol-id by creating a tiny volume and then
		// deleting it; the deleted vol-id will surface as NotFound during the
		// post-apply refresh, which is exactly what we want.
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)

			azs, err := conn.DescribeAvailabilityZones(ctx, &awsec2.DescribeAvailabilityZonesInput{})
			if err != nil || len(azs.AvailabilityZones) == 0 {
				return fmt.Errorf("could not list AZs: %w", err)
			}
			az := aws.ToString(azs.AvailabilityZones[0].ZoneName)

			cvo, err := conn.CreateVolume(ctx, &awsec2.CreateVolumeInput{
				AvailabilityZone: aws.String(az),
				Size:             aws.Int32(1),
				VolumeType:       awstypes.VolumeTypeGp2,
			})
			if err != nil {
				return err
			}
			sourceId := aws.ToString(cvo.VolumeId)

			if _, err := conn.DeleteVolume(ctx, &awsec2.DeleteVolumeInput{VolumeId: &sourceId}); err != nil {
				return err
			}
			if err := awsec2.NewVolumeDeletedWaiter(conn).Wait(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{sourceId},
			}, time.Minute); err != nil {
				return err
			}

			acctest.DatafyClient.SetRestoredVolume(dsnapId, &datafy.RestoredVolume{
				VolumeId:     sourceId,
				VolumeSizeGB: int32(size),
			})
			return nil
		}()
		if err != nil {
			panic(err)
		}
	}
}

func createDatafyReplacedByVolume(ctx context.Context, v *awstypes.Volume, replacedBy *string) func() {
	return func() {
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)

			oldVolumeId := aws.ToString(v.VolumeId)
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

// undatafyNativeVolume simulates undatafying a native volume from the datafy
// UI: a real standard volume replaces it, the datafy shard volumes are removed,
// and the mock reports the old (synthetic) volume id as replaced and unmanaged.
func undatafyNativeVolume(ctx context.Context, dv *datafyVolume, rName string) func() {
	return func() {
		err := func() error {
			conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)

			cvo, err := conn.CreateVolume(ctx, &awsec2.CreateVolumeInput{
				AvailabilityZone: dv.volumes[0].AvailabilityZone,
				Size:             aws.Int32(100),
				TagSpecifications: []awstypes.TagSpecification{
					{
						ResourceType: awstypes.ResourceTypeVolume,
						Tags: []awstypes.Tag{
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

			replacement, err := tfec2.FindEBSVolumeByID(ctx, conn, aws.ToString(cvo.VolumeId))
			if err != nil {
				return err
			}

			for _, shard := range dv.volumes {
				if _, err := conn.DeleteVolume(ctx, &awsec2.DeleteVolumeInput{VolumeId: shard.VolumeId}); err != nil {
					return err
				}
			}

			acctest.DatafyClient.SetVolume(dv.volumeId, &datafy.Volume{
				Volume:     replacement,
				HasSource:  false,
				IsManaged:  false,
				IsDatafied: false,
				ReplacedBy: aws.ToString(replacement.VolumeId),
			})
			return nil
		}()
		if err != nil {
			panic(err)
		}
	}
}

type datafyVolume struct {
	volumeId string
	volumes  []awstypes.Volume
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

		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)
		dvo, err := conn.DescribeVolumes(ctx, datafy.DescribeDatafiedVolumesInput(rs.Primary.ID))
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

func testAccDatafyCheckTagExists(_ context.Context, dv *datafyVolume, key, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, v := range dv.volumes {
			if !slices.ContainsFunc(v.Tags, func(t awstypes.Tag) bool {
				return aws.ToString(t.Key) == key && aws.ToString(t.Value) == value
			}) {
				return fmt.Errorf("tag %s=%s not found on volume %s", key, value, aws.ToString(v.VolumeId))
			}
		}
		return nil
	}
}

func testAccDatafyCheckVolumeDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "aws_ebs_volume" {
				continue
			}

			if volume, _ := acctest.DatafyClient.GetVolume(rs.Primary.ID); volume != nil && volume.IsDatafied {
				dvo, err := conn.DescribeVolumes(ctx, datafy.DescribeDatafiedVolumesInput(rs.Primary.ID))
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

func testAccDatafyEBSVolumeConfig_autoscalingNativeType(rName, volumeType string) string {
	return acctest.ConfigCompose(
		acctest.ConfigAvailableAZsNoOptIn(),
		fmt.Sprintf(`
resource "aws_ebs_volume" "test" {
  availability_zone  = data.aws_availability_zones.available.names[0]
  type               = %[2]q
  size               = 100
  autoscaling_native = true

  tags = {
    Name = %[1]q
  }
}
`, rName, volumeType))
}

func testAccDatafyEBSVolumeConfig_autoscalingNative(rName string, autoscalingNative bool) string {
	return acctest.ConfigCompose(
		acctest.ConfigAvailableAZsNoOptIn(),
		fmt.Sprintf(`
resource "aws_ebs_volume" "test" {
  availability_zone  = data.aws_availability_zones.available.names[0]
  size               = 100
  autoscaling_native = %[2]t

  tags = {
    Name = %[1]q
  }
}
`, rName, autoscalingNative))
}

// testAccDatafyEBSVolumeConfig_autoscalingNative without the flag, so the only
// config change when switching between them is the flag itself.
func testAccDatafyEBSVolumeConfig_autoscalingNativeRemoved(rName string) string {
	return acctest.ConfigCompose(
		acctest.ConfigAvailableAZsNoOptIn(),
		fmt.Sprintf(`
resource "aws_ebs_volume" "test" {
  availability_zone = data.aws_availability_zones.available.names[0]
  size              = 100

  tags = {
    Name = %[1]q
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
