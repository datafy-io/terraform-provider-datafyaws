package ec2

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	"github.com/hashicorp/terraform-provider-aws/internal/slices"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// @SDKResource("aws_ebs_volume", name="EBS Volume")
// @Tags(identifierAttribute="id")
func ResourceEBSVolume() *schema.Resource {
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
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		CustomizeDiff: customdiff.Sequence(
			resourceEBSVolumeCustomizeDiff,
			verify.SetTagsDiff,
			func(ctx context.Context, diff *schema.ResourceDiff, meta interface{}) error {
				// once the volume is managed, datafy has control on the volume. And ONLY tags can be updated via terraform.
				changes := slices.Filter(diff.GetChangedKeysPrefix(""), func(s string) bool {
					return !strings.HasPrefix(s, "tags.")
				})
				if len(changes) > 0 {
					dc := meta.(*conns.AWSClient).DatafyClient()
					if datafyVolume, datafyErr := dc.GetVolume(diff.Id()); datafyErr == nil {
						if datafyVolume.IsManaged {
							return fmt.Errorf("can't modify datafied EBS Volume (%s). Changed keys: (%s)", diff.Id(), strings.Join(changes, ","))
						}
					}
				}

				return nil
			},
		),

		Schema: map[string]*schema.Schema{
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"availability_zone": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"encrypted": {
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
			"iops": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"kms_key_id": {
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
			"size": {
				Type:         schema.TypeInt,
				Optional:     true,
				Computed:     true,
				AtLeastOneOf: []string{"size", "snapshot_id"},
			},
			"snapshot_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				AtLeastOneOf: []string{"size", "snapshot_id"},
			},
			names.AttrTags:    tftags.TagsSchema(),
			names.AttrTagsAll: tftags.TagsSchemaComputed(),
			"throughput": {
				Type:         schema.TypeInt,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validation.IntBetween(125, 1000),
			},
			"type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func resourceEBSVolumeCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Conn()

	input := &ec2.CreateVolumeInput{
		AvailabilityZone:  aws.String(d.Get("availability_zone").(string)),
		ClientToken:       aws.String(id.UniqueId()),
		TagSpecifications: getTagSpecificationsIn(ctx, ec2.ResourceTypeVolume),
	}

	if value, ok := d.GetOk("encrypted"); ok {
		input.Encrypted = aws.Bool(value.(bool))
	}

	if value, ok := d.GetOk("iops"); ok {
		input.Iops = aws.Int64(int64(value.(int)))
	}

	if value, ok := d.GetOk("kms_key_id"); ok {
		input.KmsKeyId = aws.String(value.(string))
	}

	if value, ok := d.GetOk("multi_attach_enabled"); ok {
		input.MultiAttachEnabled = aws.Bool(value.(bool))
	}

	if value, ok := d.GetOk("outpost_arn"); ok {
		input.OutpostArn = aws.String(value.(string))
	}

	if value, ok := d.GetOk("size"); ok {
		input.Size = aws.Int64(int64(value.(int)))
	}

	if value, ok := d.GetOk("snapshot_id"); ok {
		input.SnapshotId = aws.String(value.(string))
	}

	if value, ok := d.GetOk("throughput"); ok {
		input.Throughput = aws.Int64(int64(value.(int)))
	}

	if value, ok := d.GetOk("type"); ok {
		input.VolumeType = aws.String(value.(string))
	}

	if snapshotId := aws.StringValue(input.SnapshotId); strings.HasPrefix(snapshotId, "dsnap-") {
		dc := meta.(*conns.AWSClient).DatafyClient()
		restoredVolume, err := dc.CreateVolumeFromSnapshot(snapshotId,
			aws.StringValue(input.AvailabilityZone), int32(aws.Int64Value(input.Iops)), int32(aws.Int64Value(input.Throughput)),
			func() map[string]string {
				tags := make(map[string]string)
				for _, ts := range input.TagSpecifications {
					if ts.ResourceType != aws.String(ec2.ResourceTypeVolume) {
						continue
					}
					for _, t := range ts.Tags {
						tags[aws.StringValue(t.Key)] = aws.StringValue(t.Value)
					}
				}
				return tags
			}(),
		)
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "creating EBS Volume from datafy snapshot (%s): %s", snapshotId, err)
		}
		d.SetId(restoredVolume.VolumeId)

		dvo, err := conn.DescribeVolumes(datafy.DescribeDatafiedVolumesInput(d.Id()))
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s): %s", d.Id(), err)
		} else if len(dvo.Volumes) == 0 {
			return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s)", d.Id())
		}

		for _, volume := range dvo.Volumes {
			datafyVolumeId := aws.StringValue(volume.VolumeId)
			if _, err := WaitVolumeCreated(ctx, conn, datafyVolumeId, d.Timeout(schema.TimeoutCreate)); err != nil {
				return sdkdiag.AppendErrorf(diags, "waiting for datafy volume (%s) of EBS Volume (%s) create: %s", datafyVolumeId, d.Id(), err)
			}
		}

		volume := dvo.Volumes[0]
		arn := arn.ARN{
			Partition: meta.(*conns.AWSClient).Partition,
			Service:   names.EC2,
			Region:    meta.(*conns.AWSClient).Region,
			AccountID: meta.(*conns.AWSClient).AccountID,
			Resource:  fmt.Sprintf("volume/%s", d.Id()),
		}
		d.Set("arn", arn.String())
		d.Set("availability_zone", volume.AvailabilityZone)
		d.Set("encrypted", volume.Encrypted)
		d.Set("iops", volume.Iops)
		d.Set("kms_key_id", volume.KmsKeyId)
		d.Set("multi_attach_enabled", volume.MultiAttachEnabled)
		d.Set("outpost_arn", func() *string {
			if volume.OutpostArn == nil {
				return nil
			}

			outputArn := strings.ReplaceAll(*volume.OutpostArn, *volume.VolumeId, d.Id())
			return &outputArn
		}())
		d.Set("size", restoredVolume.VolumeSizeGB)
		d.Set("snapshot_id", snapshotId)
		d.Set("throughput", volume.Throughput)
		d.Set("type", volume.VolumeType)

		SetTagsOut(ctx, volume.Tags)

		return diags
	}

	output, err := conn.CreateVolumeWithContext(ctx, input)

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "creating EBS Volume: %s", err)
	}

	d.SetId(aws.StringValue(output.VolumeId))

	if _, err := WaitVolumeCreated(ctx, conn, d.Id(), d.Timeout(schema.TimeoutCreate)); err != nil {
		return sdkdiag.AppendErrorf(diags, "waiting for EBS Volume (%s) create: %s", d.Id(), err)
	}

	return append(diags, resourceEBSVolumeRead(ctx, d, meta)...)
}

func resourceEBSVolumeRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Conn()

	volume, err := FindEBSVolumeByID(ctx, conn, d.Id())

	if !d.IsNewResource() && tfresource.NotFound(err) {
		volumeId := d.Id()

		// if not found on aws, it may mean we datafied it and deleted the volume
		dc := meta.(*conns.AWSClient).DatafyClient()
		if datafyVolume, datafyErr := dc.GetVolume(volumeId); datafyErr == nil {
			// if we are managing this volume, just return the state as is after updating the tags
			if datafyVolume.IsManaged {
				dvo, err := conn.DescribeVolumes(datafy.DescribeDatafiedVolumesInput(volumeId))
				if err != nil {
					return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s): %s", volumeId, err)
				} else if len(dvo.Volumes) == 0 {
					return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s)", volumeId)
				}

				SetTagsOut(ctx, datafy.RemoveDatafyTags(dvo.Volumes[0].Tags))
				return diags
			}

			// if the volume was replaced (new source due to undatafy), it means the new
			// volume is now the source volume, and we need to set the "new" values from aws
			if datafyVolume.ReplacedBy != "" {
				d.SetId(datafyVolume.ReplacedBy)
				// check if we have the snapshot id this volume was taken from
				if dsnapId := datafyVolume.GetRestoredFromSnapshotId(); dsnapId != "" {
					d.Set("snapshot_id", dsnapId)
				}

				return append(
					sdkdiag.AppendWarningf(diags, "new EBS Volume (%s) has been created to replace the undatafied EBS Volume (%s)", datafyVolume.ReplacedBy, volumeId),
					resourceEBSVolumeRead(ctx, d, meta)...,
				)
			}
		} else if datafy.NotFound(datafyErr) {
			log.Printf("[WARN] EBS Volume %s not found, removing from state", volumeId)
			d.SetId("")
			return diags
		} else {
			err = datafyErr
		}
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "reading EBS Volume (%s): %s", d.Id(), err)
	}

	arn := arn.ARN{
		Partition: meta.(*conns.AWSClient).Partition,
		Service:   ec2.ServiceName,
		Region:    meta.(*conns.AWSClient).Region,
		AccountID: meta.(*conns.AWSClient).AccountID,
		Resource:  fmt.Sprintf("volume/%s", d.Id()),
	}
	d.Set("arn", arn.String())
	d.Set("availability_zone", volume.AvailabilityZone)
	d.Set("encrypted", volume.Encrypted)
	d.Set("iops", volume.Iops)
	d.Set("kms_key_id", volume.KmsKeyId)
	d.Set("multi_attach_enabled", volume.MultiAttachEnabled)
	d.Set("outpost_arn", volume.OutpostArn)
	d.Set("size", volume.Size)

	// if the volume has a real snapshot id, take it
	if aws.StringValue(volume.SnapshotId) != "" {
		d.Set("snapshot_id", volume.SnapshotId)
	}

	d.Set("throughput", volume.Throughput)
	d.Set("type", volume.VolumeType)

	SetTagsOut(ctx, volume.Tags)

	return diags
}

func resourceEBSVolumeUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Conn()

	if d.HasChangesExcept("tags", "tags_all") {
		// once the volume is managed, datafy has control on the volume, and it can't be updated via terraform.
		// if it was replaced (new source due to undatafy), so we set the new id and the volume properties to the state
		// and give back control to terraform
		dc := meta.(*conns.AWSClient).DatafyClient()
		if datafyVolume, datafyErr := dc.GetVolume(d.Id()); datafyErr == nil {
			if datafyVolume.IsManaged {
				return sdkdiag.AppendErrorf(diags, "can't modify datafied EBS Volume (%s)", d.Id())
			}
			if datafyVolume.ReplacedBy != "" {
				diags = sdkdiag.AppendWarningf(diags, "new EBS Volume (%s) has been created to replace the undatafied EBS Volume (%s)", datafyVolume.ReplacedBy, d.Id())

				d.SetId(datafyVolume.ReplacedBy)
				if diags := resourceEBSVolumeRead(ctx, d, meta); diags.HasError() {
					return diags
				}

				return resourceEBSVolumeUpdate(ctx, d, meta)
			}
		} else if !datafy.NotFound(datafyErr) {
			return sdkdiag.AppendErrorf(diags, "modifying EBS Volume (%s): %s", d.Id(), datafyErr)
		}

		input := &ec2.ModifyVolumeInput{
			VolumeId: aws.String(d.Id()),
		}

		if d.HasChange("iops") {
			input.Iops = aws.Int64(int64(d.Get("iops").(int)))
		}

		if d.HasChange("size") {
			input.Size = aws.Int64(int64(d.Get("size").(int)))
		}

		// "If no throughput value is specified, the existing value is retained."
		// Not currently correct, so always specify any non-zero throughput value.
		// Throughput is valid only for gp3 volumes.
		if v := d.Get("throughput").(int); v > 0 && d.Get("type").(string) == ec2.VolumeTypeGp3 {
			input.Throughput = aws.Int64(int64(v))
		}

		if d.HasChange("type") {
			volumeType := d.Get("type").(string)
			input.VolumeType = aws.String(volumeType)

			// Get Iops value because in the ec2.ModifyVolumeInput API,
			// if you change the volume type to io1, io2, or gp3, the default is 3,000.
			// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_ModifyVolume.html
			if volumeType == ec2.VolumeTypeIo1 || volumeType == ec2.VolumeTypeIo2 || volumeType == ec2.VolumeTypeGp3 {
				input.Iops = aws.Int64(int64(d.Get("iops").(int)))
			}
		}

		_, err := conn.ModifyVolumeWithContext(ctx, input)

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "modifying EBS Volume (%s): %s", d.Id(), err)
		}

		if _, err := WaitVolumeUpdated(ctx, conn, d.Id(), d.Timeout(schema.TimeoutUpdate)); err != nil {
			return sdkdiag.AppendErrorf(diags, "waiting for EBS Volume (%s) update: %s", d.Id(), err)
		}
	}

	return append(diags, resourceEBSVolumeRead(ctx, d, meta)...)
}

func resourceEBSVolumeDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Conn()

	volumesIDs := []string{d.Id()}

	// once the volume is managed, datafy has control on the volume, and it can't be deleted via terraform
	// the call must go via datafy api - that will also create the snapshot if needed.
	// if it was replaced, set the new id to the state and give back control to terraform
	dc := meta.(*conns.AWSClient).DatafyClient()
	if datafyVolume, datafyErr := dc.GetVolume(d.Id()); datafyErr == nil {
		if datafyVolume.IsManaged {
			dvo, err := conn.DescribeVolumes(datafy.DescribeDatafiedVolumesInput(d.Id()))
			if err != nil {
				return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s): %s", d.Id(), err)
			} else if len(dvo.Volumes) == 0 {
				return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s)", d.Id())
			}

			if !datafyVolume.HasSource {
				volumesIDs = make([]string, 0, len(dvo.Volumes))
			}
			for _, volume := range dvo.Volumes {
				volumesIDs = append(volumesIDs, aws.StringValue(volume.VolumeId))
			}
		}
		if datafyVolume.ReplacedBy != "" {
			diags = sdkdiag.AppendWarningf(diags, "new EBS Volume (%s) has been created to replace the undatafied EBS Volume (%s)", datafyVolume.ReplacedBy, d.Id())

			d.SetId(datafyVolume.ReplacedBy)
			return resourceEBSVolumeDelete(ctx, d, meta)
		}
	} else if !datafy.NotFound(datafyErr) {
		return sdkdiag.AppendErrorf(diags, "deleting EBS Volume (%s): %s", d.Id(), datafyErr)
	}

	if d.Get("final_snapshot").(bool) {
		input := &ec2.CreateSnapshotInput{
			TagSpecifications: tagSpecificationsFromMap(ctx, d.Get("tags_all").(map[string]interface{}), ec2.ResourceTypeSnapshot),
			VolumeId:          aws.String(d.Id()),
		}

		log.Printf("[DEBUG] Creating EBS Snapshot: %s", input)
		outputRaw, err := tfresource.RetryWhenAWSErrMessageContains(ctx, 1*time.Minute,
			func() (interface{}, error) {
				return conn.CreateSnapshotWithContext(ctx, input)
			},
			errCodeSnapshotCreationPerVolumeRateExceeded, "The maximum per volume CreateSnapshot request rate has been exceeded")

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "creating EBS Snapshot (%s): %s", d.Id(), err)
		}

		snapshotID := aws.StringValue(outputRaw.(*ec2.Snapshot).SnapshotId)

		_, err = tfresource.RetryWhenAWSErrCodeEquals(ctx, d.Timeout(schema.TimeoutDelete),
			func() (interface{}, error) {
				return nil, conn.WaitUntilSnapshotCompletedWithContext(ctx, &ec2.DescribeSnapshotsInput{
					SnapshotIds: aws.StringSlice([]string{snapshotID}),
				})
			},
			errCodeResourceNotReady)

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "waiting for EBS Snapshot (%s) create: %s", snapshotID, err)
		}
	}

	for _, vid := range volumesIDs {
		log.Printf("[DEBUG] Deleting EBS Volume: %s", d.Id())
		_, err := tfresource.RetryWhenAWSErrCodeEquals(ctx, d.Timeout(schema.TimeoutDelete),
			func() (interface{}, error) {
				return conn.DeleteVolumeWithContext(ctx, &ec2.DeleteVolumeInput{
					VolumeId: aws.String(vid),
				})
			},
			errCodeVolumeInUse)

		if tfawserr.ErrCodeEquals(err, errCodeInvalidVolumeNotFound) {
			return diags
		}

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "deleting EBS Volume (%s): %s", vid, err)
		}

		if _, err := WaitVolumeDeleted(ctx, conn, vid, d.Timeout(schema.TimeoutDelete)); err != nil {
			return sdkdiag.AppendErrorf(diags, "waiting for EBS Volume (%s) delete: %s", d.Id(), err)
		}
	}

	return diags
}

func resourceEBSVolumeCustomizeDiff(_ context.Context, diff *schema.ResourceDiff, meta interface{}) error {
	iops := diff.Get("iops").(int)
	multiAttachEnabled := diff.Get("multi_attach_enabled").(bool)
	throughput := diff.Get("throughput").(int)
	volumeType := diff.Get("type").(string)

	if diff.Id() == "" {
		// Create.

		// Iops is required for io1 and io2 volumes.
		// The default for gp3 volumes is 3,000 IOPS.
		// This parameter is not supported for gp2, st1, sc1, or standard volumes.
		// Hard validation in place to return an error if IOPs are provided
		// for an unsupported storage type.
		// Reference: https://github.com/hashicorp/terraform-provider-aws/issues/12667
		switch volumeType {
		case ec2.VolumeTypeIo1, ec2.VolumeTypeIo2:
			if iops == 0 {
				return fmt.Errorf("'iops' must be set when 'type' is '%s'", volumeType)
			}

		case ec2.VolumeTypeGp3:

		default:
			if iops != 0 {
				return fmt.Errorf("'iops' must not be set when 'type' is '%s'", volumeType)
			}
		}

		// MultiAttachEnabled is supported with io1 & io2 volumes only.
		if multiAttachEnabled && volumeType != ec2.VolumeTypeIo1 && volumeType != ec2.VolumeTypeIo2 {
			return fmt.Errorf("'multi_attach_enabled' must not be set when 'type' is '%s'", volumeType)
		}

		// Throughput is valid only for gp3 volumes.
		if throughput > 0 && volumeType != ec2.VolumeTypeGp3 {
			return fmt.Errorf("'throughput' must not be set when 'type' is '%s'", volumeType)
		}
	} else {
		// Update.

		// Setting 'iops = 0' is a no-op if the volume type does not require Iops to be specified.
		if diff.HasChange("iops") && volumeType != ec2.VolumeTypeIo1 && volumeType != ec2.VolumeTypeIo2 && volumeType != ec2.VolumeTypeGp3 && iops == 0 {
			return diff.Clear("iops")
		}
	}

	return nil
}
