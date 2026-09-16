// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: MPL-2.0

// DONOTCOPY: Copying old resources spreads bad habits. Use skaff instead.

package ec2

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	awstypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/aws-sdk-go-base/v2/tfawserr"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/create"
	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	"github.com/hashicorp/terraform-provider-aws/internal/retry"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
	"github.com/hashicorp/terraform-provider-aws/names"
)

var (
	// datafyAttrs select whether and how a volume is datafied.
	datafyAttrs = []string{datafy.AttrMode, datafy.AttrPerformanceArraySize}

	// datafyMutableAttrs are the attributes a datafied volume still accepts changes to, which
	// reach Datafy as a volume modification rather than Terraform as an update.
	datafyMutableAttrs = []string{names.AttrSize, names.AttrIOPS, names.AttrThroughput}
)

// @SDKResource("aws_ebs_volume", name="EBS Volume")
// @Tags(identifierAttribute="id")
// @IdentityAttribute("id")
// @Testing(tagsTest=false)
// @Testing(existsType="github.com/aws/aws-sdk-go-v2/service/ec2/types;awstypes;awstypes.Volume")
// @Testing(preIdentityVersion="v6.41.0")
// @Testing(importIgnore="final_snapshot")
// @Testing(plannableImportAction="NoOp")
// @Testing(name="Volume")
// @Testing(generator=false)
func resourceEBSVolume() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceEBSVolumeCreate,
		ReadWithoutTimeout:   resourceEBSVolumeRead,
		UpdateWithoutTimeout: resourceEBSVolumeUpdate,
		DeleteWithoutTimeout: resourceEBSVolumeDelete,

		SchemaVersion: 1,
		StateUpgraders: []schema.StateUpgrader{
			{
				Type:    resourceEBSVolumeV0().CoreConfigSchema().ImpliedType(),
				Upgrade: ebsVolumeStateUpgradeV0,
				Version: 0,
			},
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		CustomizeDiff: customdiff.Sequence(
			resourceDatafyEBSVolumeCustomizeDiff,
			resourceEBSVolumeCustomizeDiff,
			func(_ context.Context, diff *schema.ResourceDiff, _ any) error {
				if tags, ok := diff.Get(names.AttrTags).(map[string]any); ok {
					return datafy.ValidateNoDatafyTags(tags)
				}
				return nil
			},
			func(ctx context.Context, diff *schema.ResourceDiff, meta interface{}) error {
				changes := slices.DeleteFunc(diff.GetChangedKeysPrefix(""), func(s string) bool {
					return strings.HasPrefix(s, "tags.") || slices.Contains(datafyMutableAttrs, s)
				})
				if len(changes) > 0 {
					dc := meta.(*conns.AWSClient).DatafyClient(ctx)
					if datafyVolume, err := dc.GetVolume(diff.Id()); err == nil {
						if datafyVolume.IsManaged {
							return fmt.Errorf("can't modify datafied EBS Volume (%s). Changed keys: (%s)", diff.Id(), strings.Join(changes, ","))
						}
					}
				}
				return nil
			},
		),

		Schema: map[string]*schema.Schema{
			names.AttrARN: {
				Type:     schema.TypeString,
				Computed: true,
			},
			// Do not give this a Default: volumes provisioned by provider versions that
			// predate the attribute hold nil for it in state, and a default would plan a
			// phantom "nil -> zero value" change for them.
			datafy.AttrMode: {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringInSlice(datafy.Modes, false),
				Description: fmt.Sprintf("Create the volume as a Datafy volume optimized for `%[1]s` (capacity), `%[2]s`, or `%[3]s` (both), "+
					"instead of a standard EBS volume. The performance modes are backed by an array, sized by `%[4]s`. "+
					"This attribute is immutable: it can't be set, unset, or changed on an existing volume; it can only be removed from the configuration once the volume is no longer datafied.",
					datafy.ModeAutoscaling, datafy.ModePerformance, datafy.ModeAutoscalingPerformance, datafy.AttrPerformanceArraySize),
			},
			// Sizes below the default array of two are not expressible: they are what a volume
			// with no performance optimization already has, which omitting the mode asks for.
			datafy.AttrPerformanceArraySize: {
				Type:         schema.TypeInt,
				Optional:     true,
				ValidateFunc: validation.IntInSlice(datafy.PerformanceArraySizes),
				Description: fmt.Sprintf("Number of backing volumes in the Datafy performance array, one of %[1]v. Required by, and valid only with, a `%[2]s` of "+
					"`%[3]s` or `%[4]s`. With an array, `%[5]s` and `%[6]s` state what the array delivers in total rather than per volume. "+
					"This attribute is immutable: it can't be set, unset, or changed on an existing volume; it can only be removed from the configuration once the volume is no longer datafied.",
					datafy.PerformanceArraySizes, datafy.AttrMode, datafy.ModePerformance, datafy.ModeAutoscalingPerformance,
					names.AttrIOPS, names.AttrThroughput),
			},
			names.AttrAvailabilityZone: {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			names.AttrCreateTime: {
				Type:     schema.TypeString,
				Computed: true,
			},
			names.AttrEncrypted: {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},
			"final_snapshot": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			names.AttrIOPS: {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			names.AttrKMSKeyID: {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: verify.ValidARN,
			},
			"multi_attach_enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},
			"outpost_arn": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: verify.ValidARN,
			},
			names.AttrSize: {
				Type:         schema.TypeInt,
				Optional:     true,
				Computed:     true,
				AtLeastOneOf: []string{names.AttrSize, names.AttrSnapshotID},
			},
			names.AttrSnapshotID: {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				AtLeastOneOf: []string{names.AttrSize, names.AttrSnapshotID},
			},
			names.AttrTags:    tftags.TagsSchema(),
			names.AttrTagsAll: tftags.TagsSchemaComputed(),
			names.AttrThroughput: {
				Type:         schema.TypeInt,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validation.IntBetween(125, 2000),
			},
			names.AttrType: {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"volume_initialization_rate": {
				Type:         schema.TypeInt,
				Optional:     true,
				ValidateFunc: validation.IntBetween(100, 300),
			},
		},
	}
}

func resourceEBSVolumeCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	c := meta.(*conns.AWSClient)
	conn := c.EC2Client(ctx)

	input := ec2.CreateVolumeInput{
		AvailabilityZone:  aws.String(d.Get(names.AttrAvailabilityZone).(string)),
		ClientToken:       aws.String(create.UniqueId(ctx)),
		TagSpecifications: getTagSpecificationsIn(ctx, awstypes.ResourceTypeVolume),
	}

	if value, ok := d.GetOk(names.AttrEncrypted); ok {
		input.Encrypted = aws.Bool(value.(bool))
	}

	if value, ok := d.GetOk(names.AttrIOPS); ok {
		input.Iops = aws.Int32(int32(value.(int)))
	}

	if value, ok := d.GetOk(names.AttrKMSKeyID); ok {
		input.KmsKeyId = aws.String(value.(string))
	}

	if value, ok := d.GetOk("multi_attach_enabled"); ok {
		input.MultiAttachEnabled = aws.Bool(value.(bool))
	}

	if value, ok := d.GetOk("outpost_arn"); ok {
		input.OutpostArn = aws.String(value.(string))
	}

	if value, ok := d.GetOk(names.AttrSize); ok {
		input.Size = aws.Int32(int32(value.(int)))
	}

	if value, ok := d.GetOk(names.AttrSnapshotID); ok {
		input.SnapshotId = aws.String(value.(string))
	}

	if value, ok := d.GetOk(names.AttrThroughput); ok {
		input.Throughput = aws.Int32(int32(value.(int)))
	}

	if value, ok := d.GetOk(names.AttrType); ok {
		input.VolumeType = awstypes.VolumeType(value.(string))
	}

	if value, ok := d.GetOk("volume_initialization_rate"); ok {
		input.VolumeInitializationRate = aws.Int32(int32(value.(int)))
	}

	if snapshotId := aws.ToString(input.SnapshotId); snapshotId != "" && !strings.HasPrefix(snapshotId, "dsnap-") {
		snapshot, err := findSnapshotByID(ctx, conn, snapshotId)
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "describing snapshot (%s): %s", snapshotId, err)
		}
		if dsnapId := datafy.GetDatafySnapshotId(snapshot.Tags); dsnapId != "" {
			return sdkdiag.AppendErrorf(diags, "cannot create EBS Volume from snapshot (%s): this snapshot belongs to Datafy snapshot %q. "+
				"To restore from this snapshot, use the Datafy snapshot ID (e.g. snapshot_id = %q) instead.", snapshotId, dsnapId, dsnapId)
		}
	}

	if snapshotId := aws.ToString(input.SnapshotId); strings.HasPrefix(snapshotId, "dsnap-") {
		dc := meta.(*conns.AWSClient).DatafyClient(ctx)
		restoredVolume, err := dc.CreateVolumeFromSnapshot(datafy.CreateVolumeFromSnapshotRequest{
			DatafySnapshotId: snapshotId,
			AvailabilityZone: aws.ToString(input.AvailabilityZone),
			Iops:             aws.ToInt32(input.Iops),
			Throughput:       aws.ToInt32(input.Throughput),
			Tags:             datafy.TagsFrom(input.TagSpecifications, awstypes.ResourceTypeVolume),
		})
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "creating EBS Volume from datafy snapshot (%s): %s", snapshotId, err)
		}
		d.SetId(restoredVolume.VolumeId)

		dvo, err := conn.DescribeVolumes(ctx, datafy.DescribeDatafiedVolumesInput(d.Id()))
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s): %s", d.Id(), err)
		} else if len(dvo.Volumes) == 0 {
			return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s)", d.Id())
		}

		for _, volume := range dvo.Volumes {
			datafyVolumeId := aws.ToString(volume.VolumeId)
			if _, err := waitVolumeCreated(ctx, conn, datafyVolumeId, d.Timeout(schema.TimeoutCreate)); err != nil {
				return sdkdiag.AppendErrorf(diags, "waiting for datafy volume (%s) of EBS Volume (%s) create: %s", datafyVolumeId, d.Id(), err)
			}
		}

		volume := dvo.Volumes[0]
		if err := resourceEBSVolumeFlatten(ctx, c, &volume, d); err != nil {
			return sdkdiag.AppendErrorf(diags, "reading EBS Volume (%s): %s", d.Id(), err)
		}
		d.Set(names.AttrSize, restoredVolume.VolumeSizeGB)
		d.Set(names.AttrSnapshotID, snapshotId)
		d.Set("outpost_arn", func() *string {
			if volume.OutpostArn == nil {
				return nil
			}

			outputArn := strings.ReplaceAll(*volume.OutpostArn, *volume.VolumeId, d.Id())
			return &outputArn
		}())

		return diags
	}

	if datafyMode := d.Get(datafy.AttrMode).(string); datafyMode != "" {
		arraySize := d.Get(datafy.AttrPerformanceArraySize).(int)
		if err := validateDatafyMode(datafyMode, arraySize); err != nil {
			return sdkdiag.AppendFromErr(diags, err)
		}

		dc := meta.(*conns.AWSClient).DatafyClient(ctx)
		datafyVolume, err := dc.CreateDatafiedVolume(datafy.CreateVolumeRequest{
			AvailabilityZone:     aws.ToString(input.AvailabilityZone),
			DiskSize:             int64(aws.ToInt32(input.Size)),
			Iops:                 input.Iops,
			Throughput:           input.Throughput,
			Encrypted:            input.Encrypted,
			KmsKeyId:             aws.ToString(input.KmsKeyId),
			DatafyMode:           datafyMode,
			PerformanceArraySize: int32(arraySize),
			Tags:                 datafy.TagsFrom(input.TagSpecifications, awstypes.ResourceTypeVolume),
		})
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "creating datafied EBS Volume: %s", err)
		}
		d.SetId(aws.ToString(datafyVolume.VolumeId))

		dvo, err := conn.DescribeVolumes(ctx, datafy.DescribeDatafiedVolumesInput(d.Id()))
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s): %s", d.Id(), err)
		} else if len(dvo.Volumes) == 0 {
			return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s)", d.Id())
		}

		for _, volume := range dvo.Volumes {
			datafyVolumeId := aws.ToString(volume.VolumeId)
			if _, err := waitVolumeCreated(ctx, conn, datafyVolumeId, d.Timeout(schema.TimeoutCreate)); err != nil {
				return sdkdiag.AppendErrorf(diags, "waiting for datafy volume (%s) of EBS Volume (%s) create: %s", datafyVolumeId, d.Id(), err)
			}
		}

		volume := dvo.Volumes[0]
		if err := resourceEBSVolumeFlatten(ctx, c, &volume, d); err != nil {
			return sdkdiag.AppendErrorf(diags, "reading EBS Volume (%s): %s", d.Id(), err)
		}
		d.Set(names.AttrSize, aws.ToInt32(datafyVolume.Size))
		d.Set(names.AttrIOPS, aws.ToInt32(datafyVolume.Iops))
		d.Set(names.AttrThroughput, aws.ToInt32(datafyVolume.Throughput))

		return diags
	}

	output, err := conn.CreateVolume(ctx, &input)

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "creating EBS Volume: %s", err)
	}

	d.SetId(aws.ToString(output.VolumeId))

	if _, err := waitVolumeCreated(ctx, conn, d.Id(), d.Timeout(schema.TimeoutCreate)); err != nil {
		return sdkdiag.AppendErrorf(diags, "waiting for EBS Volume (%s) create: %s", d.Id(), err)
	}

	return append(diags, resourceEBSVolumeRead(ctx, d, meta)...)
}

func resourceEBSVolumeRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	c := meta.(*conns.AWSClient)
	conn := c.EC2Client(ctx)

	volume, err := findEBSVolumeByID(ctx, conn, d.Id())

	if !d.IsNewResource() && retry.NotFound(err) {
		volumeId := d.Id()

		// if not found on aws, it may mean we datafied it and deleted the volume
		dc := meta.(*conns.AWSClient).DatafyClient(ctx)
		if datafyVolume, err := dc.GetVolume(volumeId); err == nil {
			// if the volume was replaced (new source due to undatafy), it means the new
			// volume is now the source volume, and we need to set the "new" values from aws
			if datafyVolume.ReplacedBy != "" {
				d.SetId(datafyVolume.ReplacedBy)

				return append(
					sdkdiag.AppendWarningf(diags, "new EBS Volume (%s) has been created to replace the undatafied EBS Volume (%s)", datafyVolume.ReplacedBy, volumeId),
					resourceEBSVolumeRead(ctx, d, meta)...,
				)
			}

			// if we are managing this volume, just return the state as is after updating the tags
			if datafyVolume.IsManaged {
				dvo, err := conn.DescribeVolumes(ctx, datafy.DescribeDatafiedVolumesInput(volumeId))
				if err != nil {
					return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s): %s", volumeId, err)
				} else if len(dvo.Volumes) == 0 {
					return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s)", volumeId)
				}

				d.Set(names.AttrSize, datafyVolume.Size)
				d.Set(names.AttrIOPS, datafyVolume.Iops)
				d.Set(names.AttrThroughput, datafyVolume.Throughput)

				setTagsOut(ctx, datafy.RemoveDatafyTags(dvo.Volumes[0].Tags))
				return diags
			}
		} else if !datafy.NotFound(err) {
			return sdkdiag.AppendErrorf(diags, "reading EBS Volume (%s): %s", volumeId, err)
		}
		log.Printf("[WARN] EBS Volume %s not found, removing from state", volumeId)
		d.SetId("")
		return diags
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "reading EBS Volume (%s): %s", d.Id(), err)
	}

	if err := resourceEBSVolumeFlatten(ctx, c, volume, d); err != nil {
		return sdkdiag.AppendErrorf(diags, "reading EBS Volume (%s): %s", d.Id(), err)
	}

	return diags
}

func resourceEBSVolumeFlatten(ctx context.Context, awsClient *conns.AWSClient, volume *awstypes.Volume, d *schema.ResourceData) error {
	if err := d.Set(names.AttrARN, ebsVolumeARN(ctx, awsClient, aws.ToString(volume.VolumeId))); err != nil {
		return fmt.Errorf("setting %s: %w", names.AttrARN, err)
	}
	if err := d.Set(names.AttrAvailabilityZone, volume.AvailabilityZone); err != nil {
		return fmt.Errorf("setting %s: %w", names.AttrAvailabilityZone, err)
	}
	if err := d.Set(names.AttrCreateTime, volume.CreateTime.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("setting %s: %w", names.AttrCreateTime, err)
	}
	if err := d.Set(names.AttrEncrypted, volume.Encrypted); err != nil {
		return fmt.Errorf("setting %s: %w", names.AttrEncrypted, err)
	}
	if err := d.Set(names.AttrIOPS, volume.Iops); err != nil {
		return fmt.Errorf("setting %s: %w", names.AttrIOPS, err)
	}
	if err := d.Set(names.AttrKMSKeyID, volume.KmsKeyId); err != nil {
		return fmt.Errorf("setting %s: %w", names.AttrKMSKeyID, err)
	}
	if err := d.Set("multi_attach_enabled", volume.MultiAttachEnabled); err != nil {
		return fmt.Errorf("setting multi_attach_enabled: %w", err)
	}
	if err := d.Set("outpost_arn", volume.OutpostArn); err != nil {
		return fmt.Errorf("setting outpost_arn: %w", err)
	}
	if err := d.Set(names.AttrSize, volume.Size); err != nil {
		return fmt.Errorf("setting %s: %w", names.AttrSize, err)
	}
	snapshotId := volume.SnapshotId
	if dsnapId := datafy.GetRestoredFromSnapshotId(volume.Tags); dsnapId != "" {
		snapshotId = aws.String(dsnapId)
	}
	if err := d.Set(names.AttrSnapshotID, snapshotId); err != nil {
		return fmt.Errorf("setting %s: %w", names.AttrSnapshotID, err)
	}
	if err := d.Set(names.AttrThroughput, volume.Throughput); err != nil {
		return fmt.Errorf("setting %s: %w", names.AttrThroughput, err)
	}
	if err := d.Set(names.AttrType, volume.VolumeType); err != nil {
		return fmt.Errorf("setting %s: %w", names.AttrType, err)
	}
	if err := d.Set("volume_initialization_rate", volume.VolumeInitializationRate); err != nil {
		return fmt.Errorf("setting volume_initialization_rate: %w", err)
	}

	setTagsOut(ctx, datafy.RemoveDatafyTags(volume.Tags))

	return nil
}

func resourceEBSVolumeUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Client(ctx)

	// The datafy attributes have no volume-modification semantics: the only change that
	// reaches Update is their removal on offboarding, which only rewrites state.
	if d.HasChangesExcept(append([]string{names.AttrTags, names.AttrTagsAll}, datafyAttrs...)...) {
		// once the volume is managed, datafy has control on the volume, and it can't be updated via terraform.
		// if it was replaced (new source due to undatafy), so we set the new id and the volume properties to the state
		// and give back control to terraform
		dc := meta.(*conns.AWSClient).DatafyClient(ctx)
		if datafyVolume, err := dc.GetVolume(d.Id()); err == nil {
			if datafyVolume.IsManaged {
				var modify datafy.ModifyVolumeRequest
				var cmpAttr, oldValue, newValue string
				if d.HasChange(names.AttrIOPS) {
					oldVal, newVal := d.GetChange(names.AttrIOPS)
					oldValue = strconv.Itoa(oldVal.(int))
					newValue = strconv.Itoa(newVal.(int))
					modify.Iops = aws.Int32(int32(newVal.(int)))
					cmpAttr = names.AttrIOPS
				}
				if d.HasChange(names.AttrThroughput) {
					oldVal, newVal := d.GetChange(names.AttrThroughput)
					oldValue = strconv.Itoa(oldVal.(int))
					newValue = strconv.Itoa(newVal.(int))
					modify.Throughput = aws.Int32(int32(newVal.(int)))
					cmpAttr = names.AttrThroughput
				}
				if d.HasChange(names.AttrSize) {
					oldVal, newVal := d.GetChange(names.AttrSize)
					oldValue = strconv.Itoa(oldVal.(int))
					newValue = strconv.Itoa(newVal.(int))
					modify.SizeGb = aws.Int32(int32(newVal.(int)))
					cmpAttr = names.AttrSize
				}
				if modify == (datafy.ModifyVolumeRequest{}) {
					return append(diags, resourceEBSVolumeRead(ctx, d, meta)...)
				}
				if err := dc.ModifyVolume(d.Id(), modify); err != nil {
					return sdkdiag.AppendErrorf(diags, "modifying datafied EBS Volume (%s): %s", d.Id(), err)
				}
				if err := waitDatafyVolumeModified(ctx, dc, d.Id(), cmpAttr, oldValue, newValue, d.Timeout(schema.TimeoutUpdate)); err != nil {
					return sdkdiag.AppendErrorf(diags, "waiting for datafied EBS Volume (%s) modification: %s", d.Id(), err)
				}
				return append(diags, resourceEBSVolumeRead(ctx, d, meta)...)
			}
			if datafyVolume.ReplacedBy != "" {
				diags = sdkdiag.AppendWarningf(diags, "new EBS Volume (%s) has been created to replace the undatafied EBS Volume (%s)", datafyVolume.ReplacedBy, d.Id())

				d.SetId(datafyVolume.ReplacedBy)
				if diags := resourceEBSVolumeRead(ctx, d, meta); diags.HasError() {
					return diags
				}

				return resourceEBSVolumeUpdate(ctx, d, meta)
			}
		} else if !datafy.NotFound(err) {
			return sdkdiag.AppendErrorf(diags, "modifying EBS Volume (%s): %s", d.Id(), err)
		}

		input := ec2.ModifyVolumeInput{
			VolumeId: aws.String(d.Id()),
		}

		if d.HasChange(names.AttrIOPS) {
			input.Iops = aws.Int32(int32(d.Get(names.AttrIOPS).(int)))
		}

		if d.HasChange(names.AttrSize) {
			input.Size = aws.Int32(int32(d.Get(names.AttrSize).(int)))
		}

		// "If no throughput value is specified, the existing value is retained."
		// Not currently correct, so always specify any non-zero throughput value.
		// Throughput is valid only for gp3 volumes.
		if v := d.Get(names.AttrThroughput).(int); v > 0 && d.Get(names.AttrType).(string) == string(awstypes.VolumeTypeGp3) {
			input.Throughput = aws.Int32(int32(v))
		}

		if d.HasChange(names.AttrType) {
			volumeType := awstypes.VolumeType(d.Get(names.AttrType).(string))
			input.VolumeType = volumeType

			// Get Iops value because in the ec2.ModifyVolumeInput API,
			// if you change the volume type to io1, io2, or gp3, the default is 3,000.
			// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_ModifyVolume.html
			if volumeType == awstypes.VolumeTypeIo1 || volumeType == awstypes.VolumeTypeIo2 || volumeType == awstypes.VolumeTypeGp3 {
				input.Iops = aws.Int32(int32(d.Get(names.AttrIOPS).(int)))
			}
		}

		_, err := conn.ModifyVolume(ctx, &input)

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "modifying EBS Volume (%s): %s", d.Id(), err)
		}

		if _, err := waitVolumeUpdated(ctx, conn, d.Id(), d.Timeout(schema.TimeoutUpdate)); err != nil {
			return sdkdiag.AppendErrorf(diags, "waiting for EBS Volume (%s) update: %s", d.Id(), err)
		}
	}

	return append(diags, resourceEBSVolumeRead(ctx, d, meta)...)
}

func resourceEBSVolumeDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Client(ctx)

	volumesIDs := []string{d.Id()}

	// once the volume is managed, datafy has control on the volume, and it can't be deleted via terraform
	// the call must go via datafy api - that will also create the snapshot if needed.
	// if it was replaced, set the new id to the state and give back control to terraform
	dc := meta.(*conns.AWSClient).DatafyClient(ctx)
	if datafyVolume, err := dc.GetVolume(d.Id()); err == nil {
		if datafyVolume.IsManaged {
			dvo, err := conn.DescribeVolumes(ctx, datafy.DescribeDatafiedVolumesInput(d.Id()))
			if err != nil {
				return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s): %s", d.Id(), err)
			} else if len(dvo.Volumes) == 0 {
				return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s)", d.Id())
			}

			if !datafyVolume.HasSource {
				volumesIDs = make([]string, 0, len(dvo.Volumes))
			}
			for _, volume := range dvo.Volumes {
				volumesIDs = append(volumesIDs, aws.ToString(volume.VolumeId))
			}
		}
		if datafyVolume.ReplacedBy != "" {
			diags = sdkdiag.AppendWarningf(diags, "new EBS Volume (%s) has been created to replace the undatafied EBS Volume (%s)", datafyVolume.ReplacedBy, d.Id())

			d.SetId(datafyVolume.ReplacedBy)
			return resourceEBSVolumeDelete(ctx, d, meta)
		}
	} else if !datafy.NotFound(err) {
		return sdkdiag.AppendErrorf(diags, "deleting EBS Volume (%s): %s", d.Id(), err)
	}

	if d.Get("final_snapshot").(bool) {
		input := ec2.CreateSnapshotInput{
			TagSpecifications: tagSpecificationsFromMap(ctx, d.Get(names.AttrTagsAll).(map[string]any), awstypes.ResourceTypeSnapshot),
			VolumeId:          aws.String(d.Id()),
		}

		outputRaw, err := tfresource.RetryWhenAWSErrMessageContains(ctx, 1*time.Minute,
			func(ctx context.Context) (any, error) {
				return conn.CreateSnapshot(ctx, &input)
			},
			errCodeSnapshotCreationPerVolumeRateExceeded, "The maximum per volume CreateSnapshot request rate has been exceeded")

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "creating EBS Snapshot (%s): %s", d.Id(), err)
		}

		snapshotID := aws.ToString(outputRaw.(*ec2.CreateSnapshotOutput).SnapshotId)

		_, err = tfresource.RetryWhenAWSErrCodeEquals(ctx, d.Timeout(schema.TimeoutDelete),
			func(ctx context.Context) (any, error) {
				return waitSnapshotCompleted(ctx, conn, snapshotID, d.Timeout(schema.TimeoutDelete))
			},
			errCodeResourceNotReady)

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "waiting for EBS Snapshot (%s) create: %s", snapshotID, err)
		}
	}

	for _, vid := range volumesIDs {
		log.Printf("[DEBUG] Deleting EBS Volume: %s", d.Id())
		input := ec2.DeleteVolumeInput{
			VolumeId: aws.String(vid),
		}

		_, err := tfresource.RetryWhenAWSErrCodeEquals(ctx, d.Timeout(schema.TimeoutDelete),
			func(ctx context.Context) (any, error) {
				return conn.DeleteVolume(ctx, &input)
			},
			errCodeVolumeInUse,
		)

		if tfawserr.ErrCodeEquals(err, errCodeInvalidVolumeNotFound) {
			return diags
		}

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "deleting EBS Volume (%s): %s", vid, err)
		}

		if _, err := waitVolumeDeleted(ctx, conn, vid, d.Timeout(schema.TimeoutDelete)); err != nil {
			return sdkdiag.AppendErrorf(diags, "waiting for EBS Volume (%s) delete: %s", d.Id(), err)
		}
		log.Printf("[DEBUG] Deleting EBS Volume: %s", vid)
	}

	return diags
}

// Overridable in tests to avoid 30s/10s waits.
var (
	datafyVolumeModifiedDelay      = 10 * time.Second
	datafyVolumeModifiedMinTimeout = 10 * time.Second
)

func waitDatafyVolumeModified(ctx context.Context, dc datafy.Client, id string, attr string, oldValue string, newValue string, timeout time.Duration) error {
	stateConf := &retry.StateChangeConf{
		Pending: []string{oldValue},
		Target:  []string{newValue},
		Refresh: func(ctx context.Context) (any, string, error) {
			if attr != names.AttrSize && attr != names.AttrIOPS && attr != names.AttrThroughput {
				return nil, "", fmt.Errorf("unsupported attribute: %s", attr)
			}
			volume, err := dc.GetVolume(id)
			if err != nil {
				return nil, "", err
			}
			switch attr {
			case names.AttrSize:
				return volume, strconv.Itoa(int(*volume.Size)), nil
			case names.AttrIOPS:
				return volume, strconv.Itoa(int(*volume.Iops)), nil
			case names.AttrThroughput:
				return volume, strconv.Itoa(int(*volume.Throughput)), nil
			default:
				return nil, "", fmt.Errorf("unsupported attribute: %s", attr)
			}
		},
		Timeout:    timeout,
		Delay:      datafyVolumeModifiedDelay,
		MinTimeout: datafyVolumeModifiedMinTimeout,
	}
	_, err := stateConf.WaitForStateContext(ctx)
	return err
}

// validateDatafyMode checks the mode against the array size. An array is what performance
// optimization IS, so neither attribute means anything without the other. Datafy sizes the
// array and resolves what the volume's performance means at that size — none of that
// arithmetic is repeated here.
func validateDatafyMode(datafyMode string, arraySize int) error {
	switch {
	case arraySize > 0 && !slices.Contains(datafy.PerformanceModes, datafyMode):
		return fmt.Errorf("`%s` is only valid when `%s` is %q or %q", datafy.AttrPerformanceArraySize,
			datafy.AttrMode, datafy.ModePerformance, datafy.ModeAutoscalingPerformance)
	case arraySize == 0 && slices.Contains(datafy.PerformanceModes, datafyMode):
		return fmt.Errorf("`%s` must be set when `%s` is %q", datafy.AttrPerformanceArraySize, datafy.AttrMode, datafyMode)
	}

	return nil
}

// datafyAttrsUnknown reports whether the configuration states a datafy attribute whose value is
// not resolved yet.
// datafyAttrIsSet reports whether a raw state value carries an active datafy setting. The
// legacy SDK cannot store null for a primitive, so an attribute the configuration never held
// reads back as the zero value — indistinguishable from one nobody asked for, and not a
// setting whose removal has anything to undo.
func datafyAttrIsSet(v cty.Value) bool {
	if v.IsNull() || !v.IsKnown() {
		return false
	}

	switch v.Type() {
	case cty.String:
		return v.AsString() != ""
	case cty.Number:
		n, _ := v.AsBigFloat().Float64()
		return n > 0
	}

	return false
}

func datafyAttrsUnknown(rawConfig cty.Value) bool {
	if rawConfig.IsNull() {
		return false
	}

	for _, attr := range datafyAttrs {
		if !rawConfig.Type().HasAttribute(attr) {
			continue
		}
		if v := rawConfig.GetAttr(attr); !v.IsNull() && !v.IsKnown() {
			return true
		}
	}

	return false
}

// resourceDatafyEBSVolumeCustomizeDiff holds every Datafy rule the EBS volume has, so
// resourceEBSVolumeCustomizeDiff stays the upstream function and a rebase touches neither.
func resourceDatafyEBSVolumeCustomizeDiff(ctx context.Context, diff *schema.ResourceDiff, meta any) error {
	datafyMode := diff.Get(datafy.AttrMode).(string)
	arraySize := diff.Get(datafy.AttrPerformanceArraySize).(int)

	// An array is what performance optimization IS, so the two attributes only make sense
	// together. Datafy sizes the array and resolves what the volume's performance means at that
	// size — none of that arithmetic is repeated here.
	//
	// A value interpolated from elsewhere is unknown during plan and reads back as the zero,
	// which is indistinguishable from unset; Create settles the pair once both are known.
	if !datafyAttrsUnknown(diff.GetRawConfig()) {
		if err := validateDatafyMode(datafyMode, arraySize); err != nil {
			return err
		}
	}

	// Datafy owns the volume type of its volumes (always gp3), so `type` must be left unset.
	// Check the raw config (not the planned value): `type` is Computed,
	// so the plan carries over old/API values that the user never wrote.
	if datafyMode != "" {
		if rawConfig := diff.GetRawConfig(); !rawConfig.IsNull() {
			if typeVal := rawConfig.GetAttr(names.AttrType); typeVal.IsKnown() && !typeVal.IsNull() {
				return fmt.Errorf("`%s` must not be set when `%s` is set", names.AttrType, datafy.AttrMode)
			}
		}

		// Owning it means saying so. Left unknown, the upstream rules read the type as neither
		// gp3 nor io1/io2 and refuse the `iops` and `throughput` an array has to state — a
		// configuration this resource would otherwise have no way to express, since the same
		// rules refuse the type that would satisfy them.
		if err := diff.SetNew(names.AttrType, string(awstypes.VolumeTypeGp3)); err != nil {
			return err
		}
	}

	// The datafy attributes are immutable once the volume exists: they can't be set, unset or
	// changed. The only allowed config change is removing one once the volume is no longer
	// datafied (offboarding). Volumes from provider versions that predate an attribute (nil in
	// state) stay nil. Raw values are used because with no schema default an omitted attribute
	// produces no plan diff.
	if diff.Id() != "" {
		// rawState is the prior state as Terraform stored it after the last apply,
		// i.e. what the volume currently *is*, before any plan values are merged in.
		// rawConfig is the .tf configuration exactly as the user wrote it for this
		// run, before defaults, Computed carry-over or diff customization — so an
		// attribute the user omitted is null here, which is what lets us tell
		// "not configured" apart from "configured to the zero value".
		// Both are null in edge cases (e.g. destroy has no config, import has no
		// state), hence the IsNull checks.
		// HasAttribute guards keep this working when the attribute is absent from
		// the schema (like in the vanilla AWS provider).
		if rawState, rawConfig := diff.GetRawState(), diff.GetRawConfig(); !rawState.IsNull() && !rawConfig.IsNull() {
			getVolume := sync.OnceValues(func() (*datafy.Volume, error) {
				return meta.(*conns.AWSClient).DatafyClient(ctx).GetVolume(diff.Id())
			})

			for _, attr := range datafyAttrs {
				// HasAttribute guards keep this working when the attribute is absent from
				// the schema (like in the vanilla AWS provider).
				if !rawState.Type().HasAttribute(attr) || !rawConfig.Type().HasAttribute(attr) {
					continue
				}

				stateVal := rawState.GetAttr(attr)
				configVal := rawConfig.GetAttr(attr)
				switch {
				case configVal.IsKnown() && !configVal.IsNull() && (stateVal.IsNull() || !configVal.RawEquals(stateVal)):
					return fmt.Errorf("changing `%s` of an existing EBS Volume (%s) is not allowed", attr, diff.Id())
				// A removal is only a withdrawal when the attribute carried a setting. The
				// legacy SDK stores the zero value for one the configuration never held, so
				// on a volume in a mode with no array — or on a plain EBS volume, which
				// holds the zero for both — that zero is nobody's setting to give up, and
				// reading it as one fails every plan they make.
				case configVal.IsNull() && datafyAttrIsSet(stateVal):
					if datafyVolume, err := getVolume(); err == nil && datafyVolume.IsManaged {
						return fmt.Errorf("removing `%s` from EBS Volume (%s) is not allowed while the volume is datafied", attr, diff.Id())
					}
					// Offboarding: the removal is allowed to apply. The legacy SDK writes the
					// zero value, so the attribute ends up empty or 0 in state.
				}
			}
		}
	}

	return nil
}

func resourceEBSVolumeCustomizeDiff(ctx context.Context, diff *schema.ResourceDiff, meta any) error {
	iops := diff.Get(names.AttrIOPS).(int)
	multiAttachEnabled := diff.Get("multi_attach_enabled").(bool)
	throughput := diff.Get(names.AttrThroughput).(int)
	volumeType := awstypes.VolumeType(diff.Get(names.AttrType).(string))

	if diff.Id() == "" {
		// Create.

		// Iops is required for io1 and io2 volumes.
		// The default for gp3 volumes is 3,000 IOPS.
		// This parameter is not supported for gp2, st1, sc1, or standard volumes.
		// Hard validation in place to return an error if IOPs are provided
		// for an unsupported storage type.
		// Reference: https://github.com/hashicorp/terraform-provider-aws/issues/12667
		switch volumeType {
		case awstypes.VolumeTypeIo1, awstypes.VolumeTypeIo2:
			if iops == 0 {
				return fmt.Errorf("'iops' must be set when 'type' is '%s'", volumeType)
			}

		case awstypes.VolumeTypeGp3:

		default:
			if iops != 0 {
				return fmt.Errorf("'iops' must not be set when 'type' is '%s'", volumeType)
			}
		}

		// MultiAttachEnabled is supported with io1 & io2 volumes only.
		if multiAttachEnabled && volumeType != awstypes.VolumeTypeIo1 && volumeType != awstypes.VolumeTypeIo2 {
			return fmt.Errorf("'multi_attach_enabled' must not be set when 'type' is '%s'", volumeType)
		}

		// Throughput is valid only for gp3 volumes.
		if throughput > 0 && volumeType != awstypes.VolumeTypeGp3 {
			return fmt.Errorf("'throughput' must not be set when 'type' is '%s'", volumeType)
		}

		config := diff.GetRawConfig()
		if v := config.GetAttr(names.AttrSnapshotID); v.IsKnown() && v.IsNull() {
			if v := config.GetAttr("volume_initialization_rate"); v.IsKnown() && !v.IsNull() {
				return fmt.Errorf("'volume_initialization_rate' must not be set unless 'snapshot_id' is set")
			}
		}
	} else {
		// Update.

		// Setting 'iops = 0' is a no-op if the volume type does not require Iops to be specified.
		if diff.HasChange(names.AttrIOPS) && volumeType != awstypes.VolumeTypeIo1 && volumeType != awstypes.VolumeTypeIo2 && volumeType != awstypes.VolumeTypeGp3 && iops == 0 {
			return diff.Clear(names.AttrIOPS)
		}
	}

	return nil
}

func ebsVolumeARN(ctx context.Context, c *conns.AWSClient, volumeID string) string {
	return c.RegionalARN(ctx, names.EC2, "volume/"+volumeID)
}
