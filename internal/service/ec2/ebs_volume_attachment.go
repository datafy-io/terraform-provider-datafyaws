// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: MPL-2.0

// DONOTCOPY: Copying old resources spreads bad habits. Use skaff instead.

package ec2

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/hashicorp/aws-sdk-go-base/v2/tfawserr"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/create"
	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	"github.com/hashicorp/terraform-provider-aws/internal/retry"
	inttypes "github.com/hashicorp/terraform-provider-aws/internal/types"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// @SDKResource("aws_volume_attachment", name="EBS Volume Attachment")
// @IdentityAttribute("device_name")
// @IdentityAttribute("volume_id")
// @IdentityAttribute("instance_id")
// @ImportIDHandler("volumeAttachmentImportID")
// @Testing(preIdentityVersion="v6.41.0")
// @Testing(generator=false)
// @Testing(importStateIdFunc="testAccVolumeAttachmentImportStateIDFunc")
func resourceVolumeAttachment() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceVolumeAttachmentCreate,
		ReadWithoutTimeout:   resourceVolumeAttachmentRead,
		UpdateWithoutTimeout: schema.NoopContext,
		DeleteWithoutTimeout: resourceVolumeAttachmentDelete,

		CustomizeDiff: customdiff.Sequence(
			func(ctx context.Context, diff *schema.ResourceDiff, meta interface{}) error {
				vId, _ := diff.GetChange("volume_id")

				// once the volume is managed, datafy has control on the volume, and it can't be updated via terraform.
				if changes := diff.GetChangedKeysPrefix(""); len(changes) > 0 {
					dc := meta.(*conns.AWSClient).DatafyClient(ctx)
					if datafyVolume, err := dc.GetVolume(vId.(string)); err == nil {
						if datafyVolume.IsManaged {
							return fmt.Errorf("can't modify EBS Volume Attachment (%s) of a datafid EBS Volume (%s). Changed keys: (%s)", diff.Id(), vId.(string), strings.Join(changes, ","))
						}
					}
				}
				return nil
			},
		),

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			names.AttrDeviceName: {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"force_detach": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			names.AttrInstanceID: {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			names.AttrSkipDestroy: {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"stop_instance_before_detaching": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"volume_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceVolumeAttachmentCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Client(ctx)
	deviceName := d.Get(names.AttrDeviceName).(string)
	instanceID := d.Get(names.AttrInstanceID).(string)
	volumeID := d.Get("volume_id").(string)

	_, err := findVolumeAttachment(ctx, conn, volumeID, instanceID, deviceName)

	if retry.NotFound(err) {
		// This handles the situation where the instance is created by
		// a spot request and whilst the request has been fulfilled the
		// instance is not running yet.
		if _, err := waitVolumeAttachmentInstanceReady(ctx, conn, instanceID, instanceReadyTimeout); err != nil {
			return sdkdiag.AppendErrorf(diags, "waiting for EC2 Instance (%s) to be ready: %s", instanceID, err)
		}

		dc := meta.(*conns.AWSClient).DatafyClient(ctx)
		if datafyVolume, err := dc.GetVolume(volumeID); err == nil {
			if datafyVolume.IsManaged {
				err := dc.AttachVolume(instanceID, volumeID, deviceName)
				if err != nil {
					return sdkdiag.AppendErrorf(diags, "attaching datafy managed EBS Volume (%s) to EC2 Instance (%s): %s", volumeID, instanceID, err)
				}

				dvo, err := conn.DescribeVolumes(ctx, datafy.DescribeDatafiedVolumesInput(volumeID))
				if err != nil {
					return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s) Attachement (%s): %s", volumeID, instanceID, err)
				} else if len(dvo.Volumes) == 0 {
					return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s) Attachement (%s)", volumeID, instanceID)
				}

				for _, volume := range dvo.Volumes {
					if _, err := waitDatafyVolumeAttachmentCreated(ctx, conn, aws.ToString(volume.VolumeId), instanceID, d.Timeout(schema.TimeoutCreate)); err != nil {
						return sdkdiag.AppendErrorf(diags, "waiting for EBS Volume (%s) Attachment (%s) create: %s", volumeID, instanceID, err)
					}
				}

				d.SetId(volumeAttachmentID(deviceName, volumeID, instanceID))
				return diags
			}
		} else if !datafy.NotFound(err) {
			return sdkdiag.AppendErrorf(diags, "attaching EBS Volume (%s) to EC2 Instance (%s): %s", volumeID, instanceID, err)
		}

		input := ec2.AttachVolumeInput{
			Device:     aws.String(deviceName),
			InstanceId: aws.String(instanceID),
			VolumeId:   aws.String(volumeID),
		}

		_, err := conn.AttachVolume(ctx, &input)

		if err != nil {
			return sdkdiag.AppendErrorf(diags, "attaching EBS Volume (%s) to EC2 Instance (%s): %s", volumeID, instanceID, err)
		}

		if _, err := waitVolumeAttachmentCreated(ctx, conn, volumeID, instanceID, deviceName, d.Timeout(schema.TimeoutCreate)); err != nil {
			return sdkdiag.AppendErrorf(diags, "waiting for EBS Volume (%s) Attachment (%s) create: %s", volumeID, instanceID, err)
		}

		d.SetId(volumeAttachmentID(deviceName, volumeID, instanceID))
		return append(diags, resourceVolumeAttachmentRead(ctx, d, meta)...)
	} else if err != nil {
		return sdkdiag.AppendErrorf(diags, "reading EBS Volume (%s) Attachment (%s): %s", volumeID, instanceID, err)
	}

	d.SetId(volumeAttachmentID(deviceName, volumeID, instanceID))

	return append(diags, resourceVolumeAttachmentRead(ctx, d, meta)...)
}

func resourceVolumeAttachmentRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Client(ctx)
	deviceName := d.Get(names.AttrDeviceName).(string)
	instanceID := d.Get(names.AttrInstanceID).(string)
	volumeID := d.Get("volume_id").(string)

	_, err := findVolumeAttachment(ctx, conn, volumeID, instanceID, deviceName)

	if !d.IsNewResource() && retry.NotFound(err) {
		// if not found on aws, it may mean we datafied it and deleted the volume
		dc := meta.(*conns.AWSClient).DatafyClient(ctx)
		if datafyVolume, err := dc.GetVolume(volumeID); err == nil {
			// if we are managing this volume, just return the state as is
			if datafyVolume.IsManaged {
				return diags
			}

			// if the volume was replaced (new source due to undatafy), it means the new
			// volume is now the source volume, and we need to set the "new" values from aws
			if datafyVolume.ReplacedBy != "" {
				d.SetId(volumeAttachmentID(deviceName, datafyVolume.ReplacedBy, instanceID))
				d.Set("volume_id", datafyVolume.ReplacedBy)
				return append(
					sdkdiag.AppendWarningf(diags, "new EBS Volume (%s) has been created to replace the undatafied EBS Volume (%s)", datafyVolume.ReplacedBy, volumeID),
					resourceVolumeAttachmentRead(ctx, d, meta)...,
				)
			}
		} else if !datafy.NotFound(err) {
			return sdkdiag.AppendErrorf(diags, "reading EBS Volume (%s) Attachment (%s): %s", volumeID, instanceID, err)
		}

		log.Printf("[WARN] EBS Volume Attachment %s not found, removing from state", d.Id())
		d.SetId("")
		return diags
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "reading EBS Volume (%s) Attachment (%s): %s", volumeID, instanceID, err)
	}

	return diags
}

func resourceVolumeAttachmentDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).EC2Client(ctx)

	if _, ok := d.GetOk(names.AttrSkipDestroy); ok {
		return diags
	}

	deviceName := d.Get(names.AttrDeviceName).(string)
	instanceID := d.Get(names.AttrInstanceID).(string)
	volumeID := d.Get("volume_id").(string)

	dc := meta.(*conns.AWSClient).DatafyClient(ctx)
	if datafyVolume, err := dc.GetVolume(volumeID); err == nil {
		if datafyVolume.IsManaged {
			dvo, err := conn.DescribeVolumes(ctx, datafy.DescribeDatafiedVolumesInput(volumeID))
			if err != nil {
				return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s) Attachement (%s): %s", volumeID, d.Id(), err)
			} else if len(dvo.Volumes) == 0 {
				return sdkdiag.AppendErrorf(diags, "can't find datafy volumes of EBS volume (%s) Attachement (%s)", volumeID, d.Id())
			}

			volumesToDelete := make(map[string]string)
			if datafyVolume.HasSource {
				volumesToDelete[volumeID] = deviceName
			}
			for _, volume := range dvo.Volumes {
				if len(volume.Attachments) == 0 {
					// already detached volume should be skipped
					continue
				}
				volumesToDelete[aws.ToString(volume.VolumeId)] = aws.ToString(volume.Attachments[0].Device)
			}

			err = dc.DetachVolume(instanceID, volumeID)
			if err != nil {
				return sdkdiag.AppendErrorf(diags, "detaching datafy EBS volume (%s) from EC2 Instance (%s): %s", volumeID, instanceID, err)
			}

			for id, dn := range volumesToDelete {
				if _, err := waitVolumeAttachmentDeleted(ctx, conn, id, instanceID, dn, d.Timeout(schema.TimeoutDelete)); err != nil {
					return sdkdiag.AppendErrorf(diags, "waiting for datafy EBS Volume (%s) for EBS volume (%s) Attachment (%s) delete: %s", id, volumeID, d.Id(), err)
				}
			}
			return diags
		}

		if datafyVolume.ReplacedBy != "" {
			d.SetId(volumeAttachmentID(deviceName, datafyVolume.ReplacedBy, instanceID))
			d.Set("volume_id", datafyVolume.ReplacedBy)
			return append(
				sdkdiag.AppendWarningf(diags, "new EBS Volume (%s) has been created to replace the undatafied EBS Volume (%s)", datafyVolume.ReplacedBy, volumeID),
				resourceVolumeAttachmentDelete(ctx, d, meta)...,
			)
		}
	} else if !datafy.NotFound(err) {
		return sdkdiag.AppendErrorf(diags, "deleting EBS Volume (%s) Attachment (%s): %s", volumeID, d.Id(), err)
	}

	if _, ok := d.GetOk("stop_instance_before_detaching"); ok {
		if err := stopVolumeAttachmentInstance(ctx, conn, instanceID, false, instanceStopTimeout); err != nil {
			return sdkdiag.AppendErrorf(diags, "deleting EBS Volume (%s) Attachment (%s): %s", volumeID, instanceID, err)
		}
	}

	input := ec2.DetachVolumeInput{
		Device:     aws.String(deviceName),
		Force:      aws.Bool(d.Get("force_detach").(bool)),
		InstanceId: aws.String(instanceID),
		VolumeId:   aws.String(volumeID),
	}

	log.Printf("[DEBUG] Deleting EBS Volume Attachment: %s", d.Id())
	_, err := conn.DetachVolume(ctx, &input)

	if tfawserr.ErrMessageContains(err, errCodeIncorrectState, "available") {
		return diags
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "deleting EBS Volume (%s) Attachment (%s): %s", volumeID, instanceID, err)
	}

	if _, err := waitVolumeAttachmentDeleted(ctx, conn, volumeID, instanceID, deviceName, d.Timeout(schema.TimeoutDelete)); err != nil {
		return sdkdiag.AppendErrorf(diags, "waiting for EBS Volume (%s) Attachment (%s) delete: %s", volumeID, instanceID, err)
	}

	return diags
}

func volumeAttachmentID(name, volumeID, instanceID string) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s-", name)
	fmt.Fprintf(&buf, "%s-", instanceID)
	fmt.Fprintf(&buf, "%s-", volumeID)

	return fmt.Sprintf("vai-%d", create.StringHashcode(buf.String()))
}

func stopVolumeAttachmentInstance(ctx context.Context, conn *ec2.Client, id string, force bool, timeout time.Duration) error {
	tflog.Info(ctx, "Stopping EC2 Instance", map[string]any{
		"ec2_instance_id": id,
		"force":           force,
	})
	input := ec2.StopInstancesInput{
		Force:       aws.Bool(force),
		InstanceIds: []string{id},
	}
	_, err := conn.StopInstances(ctx, &input)

	if err != nil {
		return fmt.Errorf("stopping EC2 Instance (%s): %w", id, err)
	}

	if _, err := waitVolumeAttachmentInstanceStopped(ctx, conn, id, timeout); err != nil {
		return fmt.Errorf("waiting for EC2 Instance (%s) stop: %w", id, err)
	}

	return nil
}

var _ inttypes.SDKv2ImportID = volumeAttachmentImportID{}

type volumeAttachmentImportID struct{}

func (volumeAttachmentImportID) Create(d *schema.ResourceData) string {
	return volumeAttachmentID(d.Get(names.AttrDeviceName).(string), d.Get("volume_id").(string), d.Get(names.AttrInstanceID).(string))
}

func (volumeAttachmentImportID) Parse(id string) (string, map[string]any, error) {
	idParts := strings.Split(id, ":")

	if len(idParts) != 3 || idParts[0] == "" || idParts[1] == "" || idParts[2] == "" {
		return "", nil, fmt.Errorf("Unexpected format of ID (%q), expected DEVICE_NAME:VOLUME_ID:INSTANCE_ID", id)
	}

	deviceName := idParts[0]
	volumeID := idParts[1]
	instanceID := idParts[2]

	results := map[string]any{
		names.AttrDeviceName: deviceName,
		"volume_id":          volumeID,
		names.AttrInstanceID: instanceID,
	}

	return volumeAttachmentID(deviceName, volumeID, instanceID), results, nil
}
