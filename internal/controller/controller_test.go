package controller

import (
	"reflect"
	"sort"
	"testing"

	"github.com/enix/topomatik/internal/config"
	corev1 "k8s.io/api/core/v1"
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
			node := &corev1.Node{Spec: corev1.NodeSpec{Taints: tt.nodeTaints}}

			gotUpsert, gotDelete := c.computeTaintOps(node)

			if !equalTaints(gotUpsert, tt.wantUpsert) {
				t.Errorf("upsert mismatch\n got: %+v\nwant: %+v", gotUpsert, tt.wantUpsert)
			}
			if !equalStrings(gotDelete, tt.wantDelete) {
				t.Errorf("delete keys mismatch\n got: %+v\nwant: %+v", gotDelete, tt.wantDelete)
			}
		})
	}
}

func equalTaints(a, b []corev1.Taint) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return reflect.DeepEqual(sortTaints(a), sortTaints(b))
}

func sortTaints(taints []corev1.Taint) []corev1.Taint {
	out := append([]corev1.Taint(nil), taints...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Effect < out[j].Effect
	})
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	return reflect.DeepEqual(x, y)
}
