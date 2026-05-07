package controller

import (
	"encoding/json"
	"testing"

	"github.com/enix/topomatik/internal/config"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

type nopScheduler struct{}

func (nopScheduler) Trigger()           {}
func (nopScheduler) C() <-chan struct{} { return nil }

func newTestNode(name string, labels map[string]string, taints []corev1.Taint) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       corev1.NodeSpec{Taints: taints},
	}
}

// nodePatches returns the strategic-merge-patch payloads sent against the nodes
// resource, decoded as generic JSON for structural comparison.
func nodePatches(t *testing.T, clientset *fake.Clientset) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, a := range clientset.Actions() {
		pa, ok := a.(clienttesting.PatchAction)
		if !ok || pa.GetResource().Resource != "nodes" {
			continue
		}
		if pa.GetPatchType() != types.StrategicMergePatchType {
			t.Fatalf("unexpected patch type %q", pa.GetPatchType())
		}
		var decoded map[string]any
		if err := json.Unmarshal(pa.GetPatch(), &decoded); err != nil {
			t.Fatalf("decode patch: %v (raw: %s)", err, pa.GetPatch())
		}
		out = append(out, decoded)
	}
	return out
}

func TestReconcileNode_NoChangesDoesNotPatch(t *testing.T) {
	node := newTestNode("node-1", map[string]string{"topology/zone": "eu-west"}, nil)
	clientset := fake.NewSimpleClientset(node)

	t.Setenv("NODE_NAME", "node-1")
	c, err := New(clientset, nopScheduler{},
		map[string]string{"topology/zone": "{{ .hostname.zone }}"},
		nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.discoveryData = map[string]map[string]string{"hostname": {"zone": "eu-west"}}

	if err := c.reconcileNode(); err != nil {
		t.Fatalf("reconcileNode: %v", err)
	}

	if patches := nodePatches(t, clientset); len(patches) != 0 {
		t.Errorf("expected no patch, got %d: %+v", len(patches), patches)
	}
}

func TestReconcileNode_SendsLabelAndTaintPatch(t *testing.T) {
	node := newTestNode("node-1",
		map[string]string{
			"topology/zone": "us-east",
			"keep":          "stable",
		},
		[]corev1.Taint{
			{Key: "node.kubernetes.io/unreachable", Effect: corev1.TaintEffectNoExecute},
			{Key: "stale", Value: "old", Effect: corev1.TaintEffectNoSchedule},
		},
	)
	clientset := fake.NewSimpleClientset(node)

	t.Setenv("NODE_NAME", "node-1")
	c, err := New(clientset, nopScheduler{},
		map[string]string{"topology/zone": "{{ .hostname.zone }}"},
		map[string]config.TaintTemplate{
			"stale": {Value: "fresh", Effect: "NoSchedule"},
			"new":   {Value: "x", Effect: "NoSchedule"},
		},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.discoveryData = map[string]map[string]string{"hostname": {"zone": "eu-west"}}

	if err := c.reconcileNode(); err != nil {
		t.Fatalf("reconcileNode: %v", err)
	}

	patches := nodePatches(t, clientset)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}

	wantLabels := map[string]any{"topology/zone": "eu-west"}
	if diff := cmp.Diff(wantLabels, patches[0]["metadata"].(map[string]any)["labels"]); diff != "" {
		t.Errorf("labels in patch (-want +got):\n%s", diff)
	}

	gotTaints := patches[0]["spec"].(map[string]any)["taints"].([]any)
	gotKeys := map[string]map[string]any{}
	for _, t := range gotTaints {
		entry := t.(map[string]any)
		gotKeys[entry["key"].(string)] = entry
	}

	if _, ok := gotKeys["node.kubernetes.io/unreachable"]; ok {
		t.Error("patch should not mention unmanaged taint node.kubernetes.io/unreachable")
	}
	if got, want := gotKeys["stale"]["value"], "fresh"; got != want {
		t.Errorf("stale.value: got %v, want %v", got, want)
	}
	if got, want := gotKeys["new"]["effect"], "NoSchedule"; got != want {
		t.Errorf("new.effect: got %v, want %v", got, want)
	}
}

func TestReconcileNode_TaintWithEmptyEffectSendsDeleteDirective(t *testing.T) {
	node := newTestNode("node-1", nil, []corev1.Taint{
		{Key: "managed", Value: "old", Effect: corev1.TaintEffectNoSchedule},
	})
	clientset := fake.NewSimpleClientset(node)

	t.Setenv("NODE_NAME", "node-1")
	c, err := New(clientset, nopScheduler{}, nil, map[string]config.TaintTemplate{
		"managed": {Value: "fresh", Effect: ""},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.reconcileNode(); err != nil {
		t.Fatalf("reconcileNode: %v", err)
	}

	patches := nodePatches(t, clientset)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}

	taints := patches[0]["spec"].(map[string]any)["taints"].([]any)
	if len(taints) != 1 {
		t.Fatalf("expected 1 taint entry, got %d: %+v", len(taints), taints)
	}
	entry := taints[0].(map[string]any)
	if entry["key"] != "managed" || entry["$patch"] != "delete" {
		t.Errorf("expected delete directive for managed, got %+v", entry)
	}
}

func TestReconcileNode_EmptyRenderedLabelSendsNullValue(t *testing.T) {
	node := newTestNode("node-1",
		map[string]string{"topology/zone": "old"},
		nil,
	)
	clientset := fake.NewSimpleClientset(node)

	t.Setenv("NODE_NAME", "node-1")
	c, err := New(clientset, nopScheduler{},
		map[string]string{"topology/zone": "{{ .hostname.zone }}"},
		nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.discoveryData = map[string]map[string]string{"hostname": {"zone": ""}}

	if err := c.reconcileNode(); err != nil {
		t.Fatalf("reconcileNode: %v", err)
	}

	patches := nodePatches(t, clientset)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	labels := patches[0]["metadata"].(map[string]any)["labels"].(map[string]any)
	if v, ok := labels["topology/zone"]; !ok || v != nil {
		t.Errorf("expected topology/zone -> null, got %#v (exists=%v)", v, ok)
	}
}

func TestReconcileNode_MissingNodeSwallowsError(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	t.Setenv("NODE_NAME", "ghost")
	c, err := New(clientset, nopScheduler{},
		map[string]string{"foo": "bar"},
		nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.reconcileNode(); err != nil {
		t.Errorf("expected nil error on missing node, got: %v", err)
	}
	if patches := nodePatches(t, clientset); len(patches) != 0 {
		t.Errorf("expected no patch, got %d", len(patches))
	}
}
