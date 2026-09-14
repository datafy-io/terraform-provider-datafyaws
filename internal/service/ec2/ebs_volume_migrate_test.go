// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ec2

import (
	"context"
	"fmt"
	"maps"
	"testing"

	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
	"github.com/hashicorp/terraform-provider-aws/names"
)

func TestEBSVolumeStateUpgradeV0(t *testing.T) {
	testCases := []struct {
		name     string
		rawState map[string]any
		want     map[string]any
	}{
		{
			// The migration that has to work: a volume created as native keeps being one, so
			// a configuration naming its mode plans no change at all.
			name:     "native volume gains the mode it always had",
			rawState: map[string]any{names.AttrSize: 100, attrAutoscalingNativeV0: true},
			want:     map[string]any{names.AttrSize: 100, datafy.AttrMode: datafy.ModeAutoscaling},
		},
		{
			// False is the zero the legacy SDK wrote for an attribute nobody configured, and
			// it is also what an offboarded volume holds. Neither is datafied, which the
			// mode's absence says.
			name:     "false becomes no mode",
			rawState: map[string]any{names.AttrSize: 100, attrAutoscalingNativeV0: false},
			want:     map[string]any{names.AttrSize: 100},
		},
		{
			name:     "volumes older than the attribute are untouched",
			rawState: map[string]any{names.AttrSize: 100},
			want:     map[string]any{names.AttrSize: 100},
		},
		{
			// There was no way to ask for an array, so the size is never invented.
			name:     "the array size stays unset",
			rawState: map[string]any{attrAutoscalingNativeV0: true},
			want:     map[string]any{datafy.AttrMode: datafy.ModeAutoscaling},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ebsVolumeStateUpgradeV0(context.Background(), tc.rawState, nil)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if !maps.Equal(toComparable(got), toComparable(tc.want)) {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func toComparable(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}
