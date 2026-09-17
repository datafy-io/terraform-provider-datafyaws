// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ec2

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
)

// attrAutoscalingNativeV0 is the attribute datafy_mode replaced: a bool that could only say
// whether the volume was a Datafy native volume optimized for capacity.
const attrAutoscalingNativeV0 = "autoscaling_native"

func resourceEBSVolumeV0() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			attrAutoscalingNativeV0: {
				Type:     schema.TypeBool,
				Optional: true,
			},
		},
	}
}

// ebsVolumeStateUpgradeV0 rewrites the boolean into the mode that now names it. True meant a
// native volume optimized for capacity, which is what a datafy_mode of autoscaling asks for;
// false and absent both meant a standard EBS volume, which is what the mode's own absence says.
// There was no way to ask for a performance array, so the array size stays unset.
//
// Without this the rename is not upgradable in place. The attribute would simply drop out of
// state, leaving the mode null, and a configuration naming the mode the volume has always had
// would be refused as a change to an immutable attribute.
func ebsVolumeStateUpgradeV0(_ context.Context, rawState map[string]any, _ any) (map[string]any, error) {
	if rawState == nil {
		return nil, nil
	}

	if native, ok := rawState[attrAutoscalingNativeV0].(bool); ok && native {
		rawState[datafy.AttrMode] = datafy.ModeAutoscaling
	}
	delete(rawState, attrAutoscalingNativeV0)

	return rawState, nil
}
