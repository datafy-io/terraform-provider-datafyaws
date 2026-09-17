// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ec2_test

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
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

func TestAccDatafyEC2EBSVolume_createAutoscaling(t *testing.T) {
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
				Config: testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscaling)),
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

// A volume asking for performance: the array is created datafied and its size is what turns
// performance optimization on. The replan step is the regression guard — the legacy SDK stores
// the zero value for an attribute the configuration never held, and reading that back as an
// offboarding attempt failed every plan after the first.
func TestAccDatafyEC2EBSVolume_createPerformanceArray(t *testing.T) {
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
				Config: testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModePerformance), withPerformanceArray(4, 12000, 500)),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					testAccDatafyCheckArraySize(&dv, 4),
					testAccDatafyCheckTagExists(ctx, &dv, "Name", rName),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrPerformanceArraySize, "4"),
					// What the array delivers, not the share one member was provisioned at.
					resource.TestCheckResourceAttr(resourceName, names.AttrIOPS, "12000"),
					resource.TestCheckResourceAttr(resourceName, names.AttrThroughput, "500"),
				),
			},
			{
				Config: testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModePerformance), withPerformanceArray(4, 12000, 500)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionNoop),
					},
				},
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
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config: testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscalingPerformance), withPerformanceArray(6, 18000, 750)),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					testAccDatafyCheckArraySize(&dv, 6),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrMode, datafy.ModeAutoscalingPerformance),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrPerformanceArraySize, "6"),
				),
			},
			{
				Config: testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscalingPerformance), withPerformanceArray(6, 18000, 750)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

// The array size and the performance modes only mean anything together: the array IS the
// performance optimization. Datafy sizes it and resolves what the volume's performance means
// at that size, so only the pairing is checked here.
func TestAccDatafyEC2EBSVolume_rejectPerformanceArrayPairing(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscaling), withPerformanceArray(4, 12000, 500)),
				ExpectError: regexache.MustCompile("`datafy_performance_array_size` is only valid when `datafy_mode` is"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModePerformance)),
				ExpectError: regexache.MustCompile("`datafy_performance_array_size` must be set when `datafy_mode` is \"performance\""),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscalingPerformance)),
				ExpectError: regexache.MustCompile("`datafy_performance_array_size` must be set when `datafy_mode` is \"autoscaling_performance\""),
			},
		},
	})
}

// Datafy owns the volume type of its volumes (always gp3).
func TestAccDatafyEC2EBSVolume_rejectDatafyModeWithType(t *testing.T) {
	ctx := acctest.Context(t)
	rName := sdkacctest.RandomWithPrefix(acctest.ResourcePrefix)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(ctx, t) },
		ErrorCheck:               acctest.ErrorCheck(t, names.EC2ServiceID),
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories,
		CheckDestroy:             testAccDatafyCheckVolumeDestroy(ctx),
		Steps: []resource.TestStep{
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscaling), withVolumeType("gp2")),
				ExpectError: regexache.MustCompile("`type` must not be set when `datafy_mode` is set"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscaling), withVolumeType("gp3")),
				ExpectError: regexache.MustCompile("`type` must not be set when `datafy_mode` is set"),
			},
		},
	})
}

// on a native (datafied) volume, changing the mode errors, and removing it from the
// configuration errors as long as the volume is datafied.
func TestAccDatafyEC2EBSVolume_datafyModeDatafiedNative(t *testing.T) {
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
				Config: testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscaling)),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscalingPerformance), withPerformanceArray(4, 12000, 500)),
				ExpectError: regexache.MustCompile("changing `datafy_mode` of an existing EBS Volume"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName),
				ExpectError: regexache.MustCompile("removing `datafy_mode` from EBS Volume .* is not allowed while the volume is datafied"),
			},
		},
	})
}

// a native volume that was undatafied from the UI (replaced by a standard volume).
// Changing the mode still errors, but removing it from the configuration is allowed - it
// applies in-place, leaving the empty string in state (offboarding; the legacy Plugin SDK
// writes the zero value for removed optional primitives, not null).
func TestAccDatafyEC2EBSVolume_datafyModeUndatafiedFromUI(t *testing.T) {
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
				Config: testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscaling)),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrMode, datafy.ModeAutoscaling),
				),
			},
			{
				PreConfig:   undatafyNativeVolume(ctx, &dv, rName),
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscalingPerformance), withPerformanceArray(4, 12000, 500)),
				ExpectError: regexache.MustCompile("changing `datafy_mode` of an existing EBS Volume"),
			},
			{
				Config: testAccDatafyEBSVolumeConfig(rName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, datafy.AttrMode, ""),
				),
			},
			{
				Config: testAccDatafyEBSVolumeConfig(rName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

// a standard volume that was datafied from the UI. Naming a mode for it errors: the volume
// already exists, and datafy owns it.
func TestAccDatafyEC2EBSVolume_datafyModeDatafied(t *testing.T) {
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
				Config: testAccDatafyEBSVolumeConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
				),
			},
			{
				PreConfig:   createDatafyVolume(ctx, &v),
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscaling)),
				ExpectError: regexache.MustCompile("changing `datafy_mode` of an existing EBS Volume"),
			},
		},
	})
}

// A volume created without the attributes holds nil for them in state. Adding either
// afterwards errors; keeping them omitted stays a noop.
func TestAccDatafyEC2EBSVolume_datafyModeAddToExisting(t *testing.T) {
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
				Config: testAccDatafyEBSVolumeConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVolumeExists(ctx, t, resourceName, &v),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscaling)),
				ExpectError: regexache.MustCompile("changing `datafy_mode` of an existing EBS Volume"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModePerformance), withPerformanceArray(4, 12000, 500)),
				ExpectError: regexache.MustCompile("changing `datafy_mode` of an existing EBS Volume"),
			},
			{
				Config: testAccDatafyEBSVolumeConfig(rName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

// Both attributes are decided when the volume is created. Changing either is refused, and so
// is dropping one while the volume is datafied.
func TestAccDatafyEC2EBSVolume_performanceArrayImmutable(t *testing.T) {
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
				Config: testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscalingPerformance), withPerformanceArray(4, 12000, 500)),
				Check: resource.ComposeTestCheckFunc(
					testAccDatafyCheckVolumeExists(ctx, resourceName, &dv),
					resource.TestCheckResourceAttr(resourceName, datafy.AttrPerformanceArraySize, "4"),
				),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscalingPerformance), withPerformanceArray(8, 24000, 1000)),
				ExpectError: regexache.MustCompile("changing `datafy_performance_array_size` of an existing EBS Volume"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModePerformance), withPerformanceArray(4, 12000, 500)),
				ExpectError: regexache.MustCompile("changing `datafy_mode` of an existing EBS Volume"),
			},
			{
				// Dropping the array without dropping the mode is not a configuration at all.
				Config:      testAccDatafyEBSVolumeConfig(rName, withDatafyMode(datafy.ModeAutoscalingPerformance)),
				ExpectError: regexache.MustCompile("`datafy_performance_array_size` must be set when `datafy_mode` is"),
			},
			{
				Config:      testAccDatafyEBSVolumeConfig(rName),
				ExpectError: regexache.MustCompile("removing `datafy_(mode|performance_array_size)` from EBS Volume .* is not allowed while the volume is datafied"),
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
		conn := acctest.Provider.Meta().(*conns.AWSClient).EC2Client(ctx)

		o, err := conn.DescribeAvailabilityZones(ctx, &awsec2.DescribeAvailabilityZonesInput{})
		if err != nil {
			panic(err)
		}

		volume, err := conn.CreateVolume(ctx, &awsec2.CreateVolumeInput{
			AvailabilityZone: o.AvailabilityZones[0].ZoneName,
			Size:             aws.Int32(1),
			VolumeType:       "gp2",
			Encrypted:        aws.Bool(true),
			KmsKeyId:         aws.String("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		})
		if err != nil {
			panic(err)
		}

		acctest.DatafyClient.SetRestoredVolume(dsnapId, &datafy.RestoredVolume{
			VolumeId:     aws.ToString(volume.VolumeId),
			VolumeSizeGB: int32(size),
		})
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

func testAccDatafyCheckTagExists(ctx context.Context, dv *datafyVolume, key, value string) resource.TestCheckFunc {
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
	return fmt.Sprintf("  %-*s = %s", len(datafy.AttrPerformanceArraySize), name, value)
}

func withVolumeType(volumeType string) string {
	return attr(names.AttrType, strconv.Quote(volumeType))
}

func withDatafyMode(datafyMode string) string {
	return attr(datafy.AttrMode, strconv.Quote(datafyMode))
}

// withPerformanceArray sizes the array and states the performance it has to deliver. Both are
// ARRAY TOTALS, so they scale with the member count rather than being per-volume numbers.
func withPerformanceArray(arraySize, iops, throughput int) string {
	return strings.Join([]string{
		attr(datafy.AttrPerformanceArraySize, strconv.Itoa(arraySize)),
		attr(names.AttrIOPS, strconv.Itoa(iops)),
		attr(names.AttrThroughput, strconv.Itoa(throughput)),
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
