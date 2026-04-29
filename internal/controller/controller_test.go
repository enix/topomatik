package controller

import (
	"reflect"
	"sort"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/enix/topomatik/internal/config"
	corev1 "k8s.io/api/core/v1"
)

func newTestController(t *testing.T, templates map[string]config.TaintTemplate, data map[string]map[string]string) *Controller {
	t.Helper()
	c := &Controller{
		taintTemplates: map[string]*taintTemplate{},
		discoveryData:  data,
	}
	for key, tmpl := range templates {
		parsed := template.New("taint:" + key).Funcs(sprig.FuncMap()).Option("missingkey=error")
		if _, err := parsed.Parse(tmpl.Value); err != nil {
			t.Fatalf("parse template %q: %v", key, err)
		}
		c.taintTemplates[key] = &taintTemplate{
			value:  parsed,
			effect: tmpl.Effect,
		}
	}
	return c
}

func TestComputeDesiredTaints(t *testing.T) {
	tests := []struct {
		name        string
		templates   map[string]config.TaintTemplate
		data        map[string]map[string]string
		nodeTaints  []corev1.Taint
		wantTaints  []corev1.Taint
		wantChanged int
	}{
		{
			name: "no templates returns nil and leaves taints untouched",
			nodeTaints: []corev1.Taint{
				{Key: "foo", Value: "bar", Effect: corev1.TaintEffectNoSchedule},
			},
			wantTaints:  nil,
			wantChanged: 0,
		},
		{
			name: "adds new managed taint",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "{{ .hostname.zone }}", Effect: corev1.TaintEffectNoSchedule},
			},
			data: map[string]map[string]string{
				"hostname": {"zone": "eu-west"},
			},
			wantTaints: []corev1.Taint{
				{Key: "specialized", Value: "eu-west", Effect: corev1.TaintEffectNoSchedule},
			},
			wantChanged: 1,
		},
		{
			name: "no-op when managed taint already matches",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "{{ .hostname.zone }}", Effect: corev1.TaintEffectNoSchedule},
			},
			data: map[string]map[string]string{
				"hostname": {"zone": "eu-west"},
			},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "eu-west", Effect: corev1.TaintEffectNoSchedule},
			},
			wantTaints: []corev1.Taint{
				{Key: "specialized", Value: "eu-west", Effect: corev1.TaintEffectNoSchedule},
			},
			wantChanged: 0,
		},
		{
			name: "updates managed taint when value changes",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "{{ .hostname.zone }}", Effect: corev1.TaintEffectNoSchedule},
			},
			data: map[string]map[string]string{
				"hostname": {"zone": "eu-west"},
			},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "us-east", Effect: corev1.TaintEffectNoSchedule},
			},
			wantTaints: []corev1.Taint{
				{Key: "specialized", Value: "eu-west", Effect: corev1.TaintEffectNoSchedule},
			},
			wantChanged: 1,
		},
		{
			name: "updates managed taint when effect changes",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "fixed", Effect: corev1.TaintEffectNoExecute},
			},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "fixed", Effect: corev1.TaintEffectNoSchedule},
			},
			wantTaints: []corev1.Taint{
				{Key: "specialized", Value: "fixed", Effect: corev1.TaintEffectNoExecute},
			},
			wantChanged: 1,
		},
		{
			name: "preserves unmanaged taints",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "fixed", Effect: corev1.TaintEffectNoSchedule},
			},
			nodeTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/unreachable", Value: "", Effect: corev1.TaintEffectNoExecute},
				{Key: "specialized", Value: "old", Effect: corev1.TaintEffectNoSchedule},
			},
			wantTaints: []corev1.Taint{
				{Key: "node.kubernetes.io/unreachable", Value: "", Effect: corev1.TaintEffectNoExecute},
				{Key: "specialized", Value: "fixed", Effect: corev1.TaintEffectNoSchedule},
			},
			wantChanged: 1,
		},
		{
			name: "sanitizes rendered values",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "@@@foo+bar.foobar----.", Effect: corev1.TaintEffectNoSchedule},
			},
			wantTaints: []corev1.Taint{
				{Key: "specialized", Value: "foo_bar.foobar", Effect: corev1.TaintEffectNoSchedule},
			},
			wantChanged: 1,
		},
		{
			name: "render error preserves existing taint",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "{{ .nope.value }}", Effect: corev1.TaintEffectNoSchedule},
			},
			data: map[string]map[string]string{},
			nodeTaints: []corev1.Taint{
				{Key: "specialized", Value: "old", Effect: corev1.TaintEffectNoSchedule},
			},
			wantTaints: []corev1.Taint{
				{Key: "specialized", Value: "old", Effect: corev1.TaintEffectNoSchedule},
			},
			wantChanged: 0,
		},
		{
			name: "render error with no existing taint skips entry",
			templates: map[string]config.TaintTemplate{
				"specialized": {Value: "{{ .nope.value }}", Effect: corev1.TaintEffectNoSchedule},
			},
			data:        map[string]map[string]string{},
			wantTaints:  nil,
			wantChanged: 0,
		},
		{
			name: "counts multiple managed changes",
			templates: map[string]config.TaintTemplate{
				"a": {Value: "x", Effect: corev1.TaintEffectNoSchedule},
				"b": {Value: "y", Effect: corev1.TaintEffectNoExecute},
			},
			wantTaints: []corev1.Taint{
				{Key: "a", Value: "x", Effect: corev1.TaintEffectNoSchedule},
				{Key: "b", Value: "y", Effect: corev1.TaintEffectNoExecute},
			},
			wantChanged: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestController(t, tt.templates, tt.data)
			node := &corev1.Node{Spec: corev1.NodeSpec{Taints: tt.nodeTaints}}

			gotTaints, gotChanged := c.computeDesiredTaints(node)

			if !equalTaints(gotTaints, tt.wantTaints) {
				t.Errorf("desired taints mismatch\n got: %+v\nwant: %+v", gotTaints, tt.wantTaints)
			}
			if gotChanged != tt.wantChanged {
				t.Errorf("changed count = %d, want %d", gotChanged, tt.wantChanged)
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
