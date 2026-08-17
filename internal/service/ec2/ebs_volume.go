// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ec2

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	awstypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/aws-sdk-go-base/v2/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
	"github.com/hashicorp/terraform-provider-aws/names"
)

var (
	datafiedModifiableAttrs = []string{names.AttrSize, names.AttrIOPS, names.AttrThroughput}
)

const (
	AttrAutoscalingNative = "autoscaling_native"
)

// @SDKResource("aws_ebs_volume", name="EBS Volume")
// @Tags(identifierAttribute="id")
// @Testing(tagsTest=false)
func resourceEBSVolume() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceEBSVolumeCreate,
		ReadWithoutTimeout:   resourceEBSVolumeRead,
		UpdateWithoutTimeout: resourceEBSVolumeUpdate,
		DeleteWithoutTimeout: resourceEBSVolumeDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		CustomizeDiff: customdiff.Sequence(
			resourceEBSVolumeCustomizeDiff,
			func(_ context.Context, diff *schema.ResourceDiff, _ any) error {
				if tags, ok := diff.Get(names.AttrTags).(map[string]any); ok {
					return datafy.ValidateNoDatafyTags(tags)
				}
				return nil
			},
			func(ctx context.Context, diff *schema.ResourceDiff, meta interface{}) error {
				changes := slices.DeleteFunc(diff.GetChangedKeysPrefix(""), func(s string) bool {
					return strings.HasPrefix(s, "tags.") || slices.Contains(datafiedModifiableAttrs, s)
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
			// Do not set `Default: false` here: volumes provisioned by provider
			// versions that predate this attribute hold nil for it in state, and a
			// default would plan a phantom "nil -> false" change for them.
			AttrAutoscalingNative: {
				Type:     schema.TypeBool,
				Optional: true,
				Description: "Create the volume as a Datafy native autoscaling volume instead of a standard EBS volume. " +
					"This flag is immutable: it can't be set, unset, or changed on an existing volume; it can only be removed from the configuration once the volume is no longer datafied.",
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
				ValidateFunc: validation.IntBetween(125, 1000),
			},
			names.AttrType: {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func resourceEBSVolumeCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Client(ctx)

	input := ec2.CreateVolumeInput{
		AvailabilityZone:  aws.String(d.Get(names.AttrAvailabilityZone).(string)),
		ClientToken:       aws.String(id.UniqueId()),
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
		restoredVolume, err := dc.CreateVolumeFromSnapshot(snapshotId,
			aws.ToString(input.AvailabilityZone), aws.ToInt32(input.Iops), aws.ToInt32(input.Throughput),
			datafy.TagsFrom(input.TagSpecifications, awstypes.ResourceTypeVolume))
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
		arn := arn.ARN{
			Partition: meta.(*conns.AWSClient).Partition(ctx),
			Service:   names.EC2,
			Region:    meta.(*conns.AWSClient).Region(ctx),
			AccountID: meta.(*conns.AWSClient).AccountID(ctx),
			Resource:  fmt.Sprintf("volume/%s", d.Id()),
		}
		d.Set(names.AttrARN, arn.String())
		d.Set(names.AttrAvailabilityZone, volume.AvailabilityZone)
		d.Set(names.AttrEncrypted, volume.Encrypted)
		d.Set(names.AttrIOPS, volume.Iops)
		d.Set(names.AttrKMSKeyID, volume.KmsKeyId)
		d.Set("multi_attach_enabled", volume.MultiAttachEnabled)
		d.Set("outpost_arn", func() *string {
			if volume.OutpostArn == nil {
				return nil
			}

			outputArn := strings.ReplaceAll(*volume.OutpostArn, *volume.VolumeId, d.Id())
			return &outputArn
		}())
		d.Set(names.AttrSize, restoredVolume.VolumeSizeGB)
		d.Set(names.AttrSnapshotID, snapshotId)
		d.Set(names.AttrThroughput, volume.Throughput)
		d.Set(names.AttrType, volume.VolumeType)

		setTagsOut(ctx, datafy.RemoveDatafyTags(volume.Tags))

		return diags
	}

	if value, ok := d.GetOk(AttrAutoscalingNative); ok && value.(bool) {
		dc := meta.(*conns.AWSClient).DatafyClient(ctx)
		datafied, err := dc.CreateDatafiedVolume(aws.ToString(input.AvailabilityZone), int64(aws.ToInt32(input.Size)),
			input.Iops, input.Throughput, input.Encrypted, aws.ToString(input.KmsKeyId),
			datafy.TagsFrom(input.TagSpecifications, awstypes.ResourceTypeVolume))
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "creating datafied EBS Volume: %s", err)
		}
		d.SetId(aws.ToString(datafied.VolumeId))

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
		arnVal := arn.ARN{
			Partition: meta.(*conns.AWSClient).Partition(ctx),
			Service:   names.EC2,
			Region:    meta.(*conns.AWSClient).Region(ctx),
			AccountID: meta.(*conns.AWSClient).AccountID(ctx),
			Resource:  fmt.Sprintf("volume/%s", d.Id()),
		}
		d.Set(names.AttrARN, arnVal.String())
		d.Set(names.AttrAvailabilityZone, volume.AvailabilityZone)
		d.Set(names.AttrEncrypted, volume.Encrypted)
		d.Set(names.AttrIOPS, volume.Iops)
		d.Set(names.AttrKMSKeyID, volume.KmsKeyId)
		d.Set("multi_attach_enabled", volume.MultiAttachEnabled)
		d.Set("outpost_arn", volume.OutpostArn)
		d.Set(names.AttrSize, aws.ToInt32(input.Size))
		d.Set(names.AttrThroughput, volume.Throughput)
		d.Set(names.AttrType, volume.VolumeType)

		setTagsOut(ctx, datafy.RemoveDatafyTags(volume.Tags))

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
	conn := meta.(*conns.AWSClient).EC2Client(ctx)

	volume, err := findEBSVolumeByID(ctx, conn, d.Id())

	if !d.IsNewResource() && tfresource.NotFound(err) {
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

	snapshotId := volume.SnapshotId
	if dsnapId := datafy.GetRestoredFromSnapshotId(volume.Tags); dsnapId != "" {
		snapshotId = aws.String(dsnapId)
	}

	arn := arn.ARN{
		Partition: meta.(*conns.AWSClient).Partition(ctx),
		Service:   names.EC2,
		Region:    meta.(*conns.AWSClient).Region(ctx),
		AccountID: meta.(*conns.AWSClient).AccountID(ctx),
		Resource:  fmt.Sprintf("volume/%s", d.Id()),
	}
	d.Set(names.AttrARN, arn.String())
	d.Set(names.AttrAvailabilityZone, volume.AvailabilityZone)
	d.Set(names.AttrCreateTime, volume.CreateTime.Format(time.RFC3339))
	d.Set(names.AttrEncrypted, volume.Encrypted)
	d.Set(names.AttrIOPS, volume.Iops)
	d.Set(names.AttrKMSKeyID, volume.KmsKeyId)
	d.Set("multi_attach_enabled", volume.MultiAttachEnabled)
	d.Set("outpost_arn", volume.OutpostArn)
	d.Set(names.AttrSize, volume.Size)
	d.Set(names.AttrSnapshotID, snapshotId)
	d.Set(names.AttrThroughput, volume.Throughput)
	d.Set(names.AttrType, volume.VolumeType)

	setTagsOut(ctx, datafy.RemoveDatafyTags(volume.Tags))

	return diags
}

func resourceEBSVolumeUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Client(ctx)

	// autoscaling_native has no volume-modification semantics: the only change that
	// reaches Update is its removal on offboarding, which only rewrites state.
	if d.HasChangesExcept(names.AttrTags, names.AttrTagsAll, AttrAutoscalingNative) {
		// once the volume is managed, datafy has control on the volume, and it can't be updated via terraform.
		// if it was replaced (new source due to undatafy), so we set the new id and the volume properties to the state
		// and give back control to terraform
		dc := meta.(*conns.AWSClient).DatafyClient(ctx)
		if datafyVolume, err := dc.GetVolume(d.Id()); err == nil {
			if datafyVolume.IsManaged {
				var sizeGb, iops, throughput *int32
				var cmpAttr, oldValue, newValue string
				if d.HasChange(names.AttrIOPS) {
					oldVal, newVal := d.GetChange(names.AttrIOPS)
					oldValue = strconv.Itoa(oldVal.(int))
					newValue = strconv.Itoa(newVal.(int))
					iops = aws.Int32(int32(newVal.(int)))
					cmpAttr = names.AttrIOPS
				}
				if d.HasChange(names.AttrThroughput) {
					oldVal, newVal := d.GetChange(names.AttrThroughput)
					oldValue = strconv.Itoa(oldVal.(int))
					newValue = strconv.Itoa(newVal.(int))
					throughput = aws.Int32(int32(newVal.(int)))
					cmpAttr = names.AttrThroughput
				}
				if d.HasChange(names.AttrSize) {
					oldVal, newVal := d.GetChange(names.AttrSize)
					oldValue = strconv.Itoa(oldVal.(int))
					newValue = strconv.Itoa(newVal.(int))
					sizeGb = aws.Int32(int32(newVal.(int)))
					cmpAttr = names.AttrSize
				}
				if sizeGb == nil && iops == nil && throughput == nil {
					return append(diags, resourceEBSVolumeRead(ctx, d, meta)...)
				}
				if err := dc.ModifyVolume(d.Id(), sizeGb, iops, throughput); err != nil {
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
			func() (any, error) {
				return conn.CreateSnapshot(ctx, &input)
			},
			errCodeSnapshotCreationPerVolumeRateExceeded, "The maximum per volume CreateSnapshot request rate has been exceeded")

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "creating EBS Snapshot (%s): %s", d.Id(), err)
		}

		snapshotID := aws.ToString(outputRaw.(*ec2.CreateSnapshotOutput).SnapshotId)

		_, err = tfresource.RetryWhenAWSErrCodeEquals(ctx, d.Timeout(schema.TimeoutDelete),
			func() (any, error) {
				waiter := ec2.NewSnapshotCompletedWaiter(conn)
				return waiter.WaitForOutput(ctx, &ec2.DescribeSnapshotsInput{
					SnapshotIds: []string{snapshotID},
				}, d.Timeout(schema.TimeoutDelete))
			},
			errCodeResourceNotReady)

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "waiting for EBS Snapshot (%s) create: %s", snapshotID, err)
		}
	}

	for _, vid := range volumesIDs {
		_, err := tfresource.RetryWhenAWSErrCodeEquals(ctx, d.Timeout(schema.TimeoutDelete),
			func() (any, error) {
				return conn.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
					VolumeId: aws.String(vid),
				})
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
		Refresh: func() (any, string, error) {
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

func resourceEBSVolumeCustomizeDiff(ctx context.Context, diff *schema.ResourceDiff, meta any) error {
	iops := diff.Get(names.AttrIOPS).(int)
	multiAttachEnabled := diff.Get("multi_attach_enabled").(bool)
	throughput := diff.Get(names.AttrThroughput).(int)
	volumeType := awstypes.VolumeType(diff.Get(names.AttrType).(string))

	// Datafy owns the volume type of native volumes (always gp3), so `type` must be left unset.
	// Check the raw config (not the planned value): `type` is Computed,
	// so the plan carries over old/API values that the user never wrote.
	if value, ok := diff.GetOk(AttrAutoscalingNative); ok && value.(bool) {
		if rawConfig := diff.GetRawConfig(); !rawConfig.IsNull() {
			if typeVal := rawConfig.GetAttr(names.AttrType); typeVal.IsKnown() && !typeVal.IsNull() {
				return fmt.Errorf("`type` must not be set when autoscaling_native is true; native volumes are always provisioned as %q", awstypes.VolumeTypeGp3)
			}
		}
	}

	// autoscaling_native is immutable once the volume exists: it can't be set,
	// unset, or flipped. The only allowed config change is removing it once the
	// volume is no longer datafied (offboarding). Volumes from provider versions
	// that predate the attribute (nil in state) stay nil. Raw values are used
	// because with no schema default an omitted attribute produces no plan diff.
	if diff.Id() != "" {
		// HasAttribute guards keep this working when the attribute is absent from
		// the schema (like in the vanilla AWS provider).
		if rawState, rawConfig := diff.GetRawState(), diff.GetRawConfig(); !rawState.IsNull() && !rawConfig.IsNull() &&
			rawState.Type().HasAttribute(AttrAutoscalingNative) && rawConfig.Type().HasAttribute(AttrAutoscalingNative) {
			stateVal := rawState.GetAttr(AttrAutoscalingNative)
			configVal := rawConfig.GetAttr(AttrAutoscalingNative)
			switch {
			case configVal.IsKnown() && !configVal.IsNull() && (stateVal.IsNull() || !configVal.RawEquals(stateVal)):
				return fmt.Errorf("changing `autoscaling_native` of an existing EBS Volume (%s) is not allowed", diff.Id())
			case configVal.IsNull() && !stateVal.IsNull():
				dc := meta.(*conns.AWSClient).DatafyClient(ctx)
				if datafyVolume, err := dc.GetVolume(diff.Id()); err == nil && datafyVolume.IsManaged {
					return fmt.Errorf("removing `autoscaling_native` from EBS Volume (%s) is not allowed while the volume is datafied", diff.Id())
				}
				// Offboarding: the removal is allowed to apply. The legacy SDK writes the
				// zero value, so the attribute ends up as false in state.
			}
		}
	}

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
	} else {
		// Update.

		// Setting 'iops = 0' is a no-op if the volume type does not require Iops to be specified.
		if diff.HasChange(names.AttrIOPS) && volumeType != awstypes.VolumeTypeIo1 && volumeType != awstypes.VolumeTypeIo2 && volumeType != awstypes.VolumeTypeGp3 && iops == 0 {
			return diff.Clear(names.AttrIOPS)
		}
	}

	return nil
}
