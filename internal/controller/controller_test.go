package controller

import (
	"encoding/json"
	"testing"

	"github.com/enix/topomatik/internal/config"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
)

var (
	sortTaintsByKey   = cmpopts.SortSlices(func(a, b corev1.Taint) bool { return a.Key < b.Key })
	sortStringsSlice  = cmpopts.SortSlices(func(a, b string) bool { return a < b })
	equateEmptySlices = cmpopts.EquateEmpty()
)

func newTestController(t *testing.T, templates map[string]config.TaintTemplate, data map[string]map[string]string) *Controller {
	t.Helper()
	c, err := New(nil, nil, nil, templates)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.discoveryData = data
	return c
}

func newTestControllerWithLabels(t *testing.T, labelTemplates map[string]string, data map[string]map[string]string) *Controller {
	t.Helper()
	c, err := New(nil, nil, labelTemplates, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.discoveryData = data
	return c
}

func TestComputeTaintOps(t *testing.T) {
	tests := []struct {
		name       string
		templates  map[string]config.TaintTemplate
		data       map[string]map[string]string
		nodeTaints []corev1.Taint
		wantUpsert []corev1.Taint
		wantDelete []string
	}{
		{
			name: "no templates returns no ops",
			nodeTaints: []corev1.Taint{
				{Key: "foo", Value: "bar", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			name: "adds new managed taint",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "{{ .hostname.zone }}", Effect: "NoSchedule"},
			},
			data: map[string]map[string]string{
				"hostname": {"zone": "eu-west"},
			},
			wantUpsert: []corev1.Taint{
				{Key: "specialized", Value: "eu-west", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			name: "no-op when managed taint already matches",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "{{ .hostname.zone }}", Effect: "NoSchedule"},
			},
			data: map[string]map[string]string{
				"hostname": {"zone": "eu-west"},
			},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "eu-west", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			name: "updates managed taint when value changes",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "{{ .hostname.zone }}", Effect: "NoSchedule"},
			},
			data: map[string]map[string]string{
				"hostname": {"zone": "eu-west"},
			},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "us-east", Effect: corev1.TaintEffectNoSchedule},
			},
			wantUpsert: []corev1.Taint{
				{Key: "specialized", Value: "eu-west", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			name: "updates managed taint when effect changes",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "fixed", Effect: "NoExecute"},
			},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "fixed", Effect: corev1.TaintEffectNoSchedule},
			},
			wantUpsert: []corev1.Taint{
				{Key: "specialized", Value: "fixed", Effect: corev1.TaintEffectNoExecute},
			},
		},
		{
			name: "templated effect renders from discovery data",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "fixed", Effect: "{{ .hostname.effect }}"},
			},
			data: map[string]map[string]string{
				"hostname": {"effect": "PreferNoSchedule"},
			},
			wantUpsert: []corev1.Taint{
				{Key: "specialized", Value: "fixed", Effect: corev1.TaintEffectPreferNoSchedule},
			},
		},
		{
			name: "ignores unmanaged taints",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "fixed", Effect: "NoSchedule"},
			},
			nodeTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/unreachable", Value: "", Effect: corev1.TaintEffectNoExecute},
				{Key: "specialized", Value: "old", Effect: corev1.TaintEffectNoSchedule},
			},
			wantUpsert: []corev1.Taint{
				{Key: "specialized", Value: "fixed", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			name: "sanitizes rendered values",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "@@@foo+bar.foobar----.", Effect: "NoSchedule"},
			},
			wantUpsert: []corev1.Taint{
				{Key: "specialized", Value: "foo_bar.foobar", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			name: "value render error skips taint",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "{{ .nope.value }}", Effect: "NoSchedule"},
			},
			data: map[string]map[string]string{},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "old", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			name: "value render error with no existing taint is a no-op",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "{{ .nope.value }}", Effect: "NoSchedule"},
			},
			data: map[string]map[string]string{},
		},
		{
			name: "effect render error skips taint",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "fixed", Effect: "{{ .nope.effect }}"},
			},
			data: map[string]map[string]string{},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "old", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			name: "invalid rendered effect skips taint",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "fixed", Effect: "Bogus"},
			},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "old", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			name: "empty effect removes existing managed taint",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "fixed", Effect: ""},
			},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "old", Effect: corev1.TaintEffectNoSchedule},
			},
			wantDelete: []string{"specialized"},
		},
		{
			name: "empty effect from template removes existing managed taint",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "fixed", Effect: "{{ .hostname.effect }}"},
			},
			data: map[string]map[string]string{
				"hostname": {"effect": ""},
			},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "old", Effect: corev1.TaintEffectNoSchedule},
			},
			wantDelete: []string{"specialized"},
		},
		{
			name: "empty effect with no existing taint is a no-op",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "fixed", Effect: ""},
			},
		},
		{
			name: "multiple managed upserts",
			templates: map[string]config.TaintTemplate{
				"a": {Value: "x", Effect: "NoSchedule"},
				"b": {Value: "y", Effect: "NoExecute"},
			},
			wantUpsert: []corev1.Taint{
				{Key: "a", Value: "x", Effect: corev1.TaintEffectNoSchedule},
				{Key: "b", Value: "y", Effect: corev1.TaintEffectNoExecute},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestController(t, tt.templates, tt.data)

			gotUpsert, gotDelete := computeTaintOps(c.taintTemplates, c.discoveryData, tt.nodeTaints)

			if diff := cmp.Diff(tt.wantUpsert, gotUpsert, sortTaintsByKey, equateEmptySlices); diff != "" {
				t.Errorf("upsert mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantDelete, gotDelete, sortStringsSlice, equateEmptySlices); diff != "" {
				t.Errorf("delete keys mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestComputeLabelPatch(t *testing.T) {
	tests := []struct {
		name        string
		templates   map[string]string
		data        map[string]map[string]string
		nodeLabels  map[string]string
		wantPatch   map[string]any
	}{
		{
			name:      "no templates returns empty patch",
			nodeLabels: map[string]string{"foo": "bar"},
			wantPatch:  map[string]any{},
		},
		{
			name: "adds new label",
			templates: map[string]string{
				"topology/zone": "{{ .hostname.zone }}",
			},
			data: map[string]map[string]string{
				"hostname": {"zone": "eu-west"},
			},
			wantPatch: map[string]any{"topology/zone": "eu-west"},
		},
		{
			name: "no-op when label already matches",
			templates: map[string]string{
				"topology/zone": "{{ .hostname.zone }}",
			},
			data: map[string]map[string]string{
				"hostname": {"zone": "eu-west"},
			},
			nodeLabels: map[string]string{"topology/zone": "eu-west"},
			wantPatch:  map[string]any{},
		},
		{
			name: "updates label when value changes",
			templates: map[string]string{
				"topology/zone": "{{ .hostname.zone }}",
			},
			data: map[string]map[string]string{
				"hostname": {"zone": "eu-west"},
			},
			nodeLabels: map[string]string{"topology/zone": "us-east"},
			wantPatch:  map[string]any{"topology/zone": "eu-west"},
		},
		{
			name: "empty rendered value removes existing label",
			templates: map[string]string{
				"topology/zone": "{{ .hostname.zone }}",
			},
			data: map[string]map[string]string{
				"hostname": {"zone": ""},
			},
			nodeLabels: map[string]string{"topology/zone": "old"},
			wantPatch:  map[string]any{"topology/zone": nil},
		},
		{
			name: "empty rendered value with no existing label is a no-op",
			templates: map[string]string{
				"topology/zone": "{{ .hostname.zone }}",
			},
			data: map[string]map[string]string{
				"hostname": {"zone": ""},
			},
			wantPatch: map[string]any{},
		},
		{
			name: "render error skips label",
			templates: map[string]string{
				"topology/zone": "{{ .nope.zone }}",
			},
			data:       map[string]map[string]string{},
			nodeLabels: map[string]string{"topology/zone": "old"},
			wantPatch:  map[string]any{},
		},
		{
			name: "sanitizes rendered value",
			templates: map[string]string{
				"topology/zone": "@@@foo+bar.foobar----.",
			},
			wantPatch: map[string]any{"topology/zone": "foo_bar.foobar"},
		},
		{
			name: "sanitized value that becomes empty removes existing label",
			templates: map[string]string{
				"topology/zone": "@@@",
			},
			nodeLabels: map[string]string{"topology/zone": "old"},
			wantPatch:  map[string]any{"topology/zone": nil},
		},
		{
			name: "multiple labels mixed operations",
			templates: map[string]string{
				"add":    "{{ .h.a }}",
				"keep":   "{{ .h.b }}",
				"remove": "{{ .h.empty }}",
			},
			data: map[string]map[string]string{
				"h": {"a": "new", "b": "stable", "empty": ""},
			},
			nodeLabels: map[string]string{
				"keep":   "stable",
				"remove": "old",
			},
			wantPatch: map[string]any{
				"add":    "new",
				"remove": nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestControllerWithLabels(t, tt.templates, tt.data)

			got := computeLabelPatch(c.labelTemplates, c.discoveryData, tt.nodeLabels)

			if diff := cmp.Diff(tt.wantPatch, got); diff != "" {
				t.Errorf("label patch mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildNodeStrategicMergePatch(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]any
		upsert      []corev1.Taint
		deleteKeys  []string
		wantJSON    string
	}{
		{
			name:     "all empty produces empty patch",
			wantJSON: `{}`,
		},
		{
			name:     "labels only",
			labels:   map[string]any{"foo": "bar"},
			wantJSON: `{"metadata":{"labels":{"foo":"bar"}}}`,
		},
		{
			name:     "label removal renders as null",
			labels:   map[string]any{"foo": nil},
			wantJSON: `{"metadata":{"labels":{"foo":null}}}`,
		},
		{
			name: "upsert taints only",
			upsert: []corev1.Taint{
				{Key: "k", Value: "v", Effect: corev1.TaintEffectNoSchedule},
			},
			wantJSON: `{"spec":{"taints":[{"key":"k","value":"v","effect":"NoSchedule"}]}}`,
		},
		{
			name:       "delete taints only use $patch directive",
			deleteKeys: []string{"gone"},
			wantJSON:   `{"spec":{"taints":[{"$patch":"delete","key":"gone"}]}}`,
		},
		{
			name:   "labels + upsert + delete combined",
			labels: map[string]any{"foo": "bar"},
			upsert: []corev1.Taint{
				{Key: "k", Value: "v", Effect: corev1.TaintEffectNoSchedule},
			},
			deleteKeys: []string{"gone"},
			wantJSON:   `{"metadata":{"labels":{"foo":"bar"}},"spec":{"taints":[{"key":"k","value":"v","effect":"NoSchedule"},{"$patch":"delete","key":"gone"}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildNodeStrategicMergePatch(tt.labels, tt.upsert, tt.deleteKeys)
			if err != nil {
				t.Fatalf("buildNodeStrategicMergePatch: %v", err)
			}

			var gotAny, wantAny any
			if err := json.Unmarshal(got, &gotAny); err != nil {
				t.Fatalf("unmarshal got: %v (raw: %s)", err, got)
			}
			if err := json.Unmarshal([]byte(tt.wantJSON), &wantAny); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}

			if diff := cmp.Diff(wantAny, gotAny); diff != "" {
				t.Errorf("patch mismatch (-want +got):\n%s\nraw got: %s", diff, got)
			}
		})
	}
}
