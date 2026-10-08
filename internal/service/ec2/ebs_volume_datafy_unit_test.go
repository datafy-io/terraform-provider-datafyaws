package ec2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/go-cty/cty/gocty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/hashicorp/terraform-provider-aws/internal/datafy"
)

// newTestDatafyClientForWaiter creates a Datafy client pointed at a mock HTTP server.
func newTestDatafyClientForWaiter(t *testing.T, handler http.HandlerFunc) (datafy.Client, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	client := datafy.NewDatafyClient(server.URL, "test-token")
	return client, server.Close
}

// overrideDatafyWaitTiming replaces the polling delays with short values for the duration of the test.
func overrideDatafyWaitTiming(t *testing.T) {
	t.Helper()
	origDelay, origMin := datafyVolumeModifiedDelay, datafyVolumeModifiedMinTimeout
	datafyVolumeModifiedDelay = 10 * time.Millisecond
	datafyVolumeModifiedMinTimeout = 10 * time.Millisecond
	t.Cleanup(func() {
		datafyVolumeModifiedDelay = origDelay
		datafyVolumeModifiedMinTimeout = origMin
	})
}

func volumeStatusBodyWithAttrs(size int, iops int, throughput int) []byte {
	b, _ := json.Marshal(map[string]any{
		"volumeId":   "vol-abc123",
		"isManaged":  true,
		"sizeBytes":  int64(size) * 1024 * 1024 * 1024,
		"iops":       iops,
		"throughput": throughput,
	})
	return b
}

// TestWaitDatafyVolumeModified_modifyingThenChanged verifies the happy path: volume returns
// old values for the first two polls and then transitions to new values.
func TestWaitDatafyVolumeModified_modifyingThenChanged(t *testing.T) {
	overrideDatafyWaitTiming(t)

	var callCount atomic.Int32
	// Simulate size changing from 10 to 20 after 2 polls
	client, cleanup := newTestDatafyClientForWaiter(t, func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		if count <= 2 {
			w.Write(volumeStatusBodyWithAttrs(10, 100, 200))
		} else {
			w.Write(volumeStatusBodyWithAttrs(20, 100, 200))
		}
	})
	defer cleanup()

	err := waitDatafyVolumeModified(context.Background(), client, "vol-abc123", "size", "10", "20", 5*time.Second)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// Must have polled at least 3 times: 2 old value + 1 new value.
	if n := callCount.Load(); n < 3 {
		t.Errorf("expected at least 3 polls, got %d", n)
	}
}

// TestWaitDatafyVolumeModified_alreadyChanged verifies success when the volume is already
// at the new value on the first poll.
func TestWaitDatafyVolumeModified_alreadyChanged(t *testing.T) {
	overrideDatafyWaitTiming(t)

	var callCount atomic.Int32
	// Already at new value (simulate size=20)
	client, cleanup := newTestDatafyClientForWaiter(t, func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write(volumeStatusBodyWithAttrs(20, 100, 200))
	})
	defer cleanup()

	err := waitDatafyVolumeModified(context.Background(), client, "vol-abc123", "size", "10", "20", 5*time.Second)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if n := callCount.Load(); n < 1 {
		t.Errorf("expected exactly 1 poll, got %d", n)
	}
}

// TestWaitDatafyVolumeModified_timeout verifies that the function returns an error when the
// volume stays in MODIFYING for longer than the provided timeout.
func TestWaitDatafyVolumeModified_timeout(t *testing.T) {
	overrideDatafyWaitTiming(t)

	// Always returns old value (size=10), never transitions
	client, cleanup := newTestDatafyClientForWaiter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(volumeStatusBodyWithAttrs(10, 100, 200))
	})
	defer cleanup()

	err := waitDatafyVolumeModified(context.Background(), client, "vol-abc123", "size", "10", "20", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
}

// TestWaitDatafyVolumeModified_apiError verifies that a GetVolume error is propagated and
// aborts the wait immediately.
func TestWaitDatafyVolumeModified_apiError(t *testing.T) {
	overrideDatafyWaitTiming(t)

	client, cleanup := newTestDatafyClientForWaiter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"message": "internal server error"})
	})
	defer cleanup()

	err := waitDatafyVolumeModified(context.Background(), client, "vol-abc123", "size", "10", "20", 5*time.Second)
	if err == nil {
		t.Fatal("expected an error from the API, got nil")
	}
}

func TestDatafyAttrIsSet(t *testing.T) {
	testCases := []struct {
		name string
		val  cty.Value
		want bool
	}{
		{name: "null bool", val: cty.NullVal(cty.Bool)},
		{name: "null number", val: cty.NullVal(cty.Number)},
		{name: "unknown", val: cty.UnknownVal(cty.Number)},
		// The zero the legacy SDK writes for an attribute the configuration never held: a
		// volume with no performance array, or a plain EBS volume, which holds it for all three.
		{name: "false", val: cty.False},
		{name: "zero", val: cty.NumberIntVal(0)},
		{name: "true", val: cty.True, want: true},
		{name: "performance tier", val: cty.NumberIntVal(4), want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := datafyAttrIsSet(tc.val); got != tc.want {
				t.Fatalf("expected %t, got %t", tc.want, got)
			}
		})
	}
}

// TestDatafyCustomizeDiffCreatePlan runs the two CustomizeDiffs in the order the resource does,
// against a create plan. It exists because the Datafy rules and the upstream ones can contradict
// each other: Datafy refuses an explicit `type`, and the upstream rules refuse the `iops` and
// `throughput` an array must state unless the type is gp3. Nothing in the resource's own
// validation catches that pair — only planning does.
//
// The configuration carries a real cty value, so the rules that read the raw config are
// exercised too rather than skipped.
func TestDatafyCustomizeDiffCreatePlan(t *testing.T) {
	resource := &schema.Resource{
		Schema: ResourceEBSVolume().Schema,
		CustomizeDiff: customdiff.Sequence(
			resourceDatafyEBSVolumeCustomizeDiff,
			resourceEBSVolumeCustomizeDiff,
		),
	}

	testCases := []struct {
		name     string
		config   map[string]any
		wantType string
		wantErr  string
	}{
		{
			name: "autoscaling states no performance",
			config: map[string]any{
				"availability_zone":    "us-east-1a",
				"size":                 100,
				datafy.AttrAutoscaling: true,
			},
			wantType: ec2.VolumeTypeGp3,
		},
		{
			name: "a performance array states both",
			config: map[string]any{
				"availability_zone":        "us-east-1a",
				"size":                     100,
				datafy.AttrPerformance:     true,
				datafy.AttrPerformanceTier: 4,
				"iops":                     12000,
				"throughput":               500,
			},
			wantType: ec2.VolumeTypeGp3,
		},
		{
			name: "capacity and performance",
			config: map[string]any{
				"availability_zone":        "us-east-1a",
				"size":                     100,
				datafy.AttrAutoscaling:     true,
				datafy.AttrPerformance:     true,
				datafy.AttrPerformanceTier: 8,
				"iops":                     24000,
				"throughput":               1000,
			},
			wantType: ec2.VolumeTypeGp3,
		},
		{
			name: "a tier needs the performance flag",
			config: map[string]any{
				"availability_zone":        "us-east-1a",
				"size":                     100,
				datafy.AttrAutoscaling:     true,
				datafy.AttrPerformanceTier: 4,
			},
			wantErr: "`datafy_performance_tier` is only valid when `datafy_performance` is true",
		},
		{
			name: "performance needs a tier",
			config: map[string]any{
				"availability_zone":    "us-east-1a",
				"size":                 100,
				datafy.AttrPerformance: true,
			},
			wantErr: "`datafy_performance_tier` must be set when `datafy_performance` is true",
		},
		{
			// Without a datafy flag the upstream rules are untouched: a plain volume still
			// may not state iops without saying which type can carry them.
			name: "a plain volume keeps the upstream rules",
			config: map[string]any{
				"availability_zone": "us-east-1a",
				"size":              100,
				"iops":              12000,
			},
			wantErr: "'iops' must not be set when 'type' is ''",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			state, config := datafyTestConfig(t, resource, tc.config)
			instanceDiff, err := resource.Diff(context.Background(), state, config, nil)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got none", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got := instanceDiff.Attributes["type"].New; got != tc.wantType {
				t.Fatalf("expected planned type %q, got %q", tc.wantType, got)
			}
		})
	}
}

// datafyTestConfig builds the configuration a plan is made from. The value has to be a real
// cty object: CustomizeDiff reads attributes straight off the raw config, and one built without
// a cty value panics as soon as anything asks for an attribute.
func datafyTestConfig(t *testing.T, r *schema.Resource, raw map[string]any) (*terraform.InstanceState, *terraform.ResourceConfig) {
	t.Helper()

	block := r.CoreConfigSchema()
	vals := make(map[string]cty.Value)
	for name, attrTy := range block.ImpliedType().AttributeTypes() {
		v, ok := raw[name]
		if !ok {
			vals[name] = cty.NullVal(attrTy)
			continue
		}

		cv, err := gocty.ToCtyValue(v, attrTy)
		if err != nil {
			t.Fatalf("%s: %s", name, err)
		}
		vals[name] = cv
	}

	configVal := cty.ObjectVal(vals)

	// The raw config reaches CustomizeDiff through the prior state, not through the
	// ResourceConfig, and an empty ID still plans as a create.
	return &terraform.InstanceState{RawConfig: configVal},
		terraform.NewResourceConfigShimmed(configVal, block)
}
