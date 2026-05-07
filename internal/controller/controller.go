package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/enix/topomatik/internal/autodiscovery"
	"github.com/enix/topomatik/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

var labelRegexp = regexp.MustCompile(`[^A-Za-z0-9._-]`)

type ReconciliationScheduler interface {
	Trigger()
	C() <-chan struct{}
}

type taintTemplate struct {
	value  *template.Template
	effect *template.Template
}

type Controller struct {
	nodeName       string
	clientset      kubernetes.Interface
	labelTemplates map[string]*template.Template
	taintTemplates map[string]*taintTemplate
	engines        []*autodiscovery.Engine
	discoveryData  map[string]map[string]string
	scheduler      ReconciliationScheduler
}

func New(clientset kubernetes.Interface, scheduler ReconciliationScheduler, labelTemplates map[string]string, taintTemplates map[string]config.TaintTemplate) (*Controller, error) {
	parsedLabels, err := parseLabelTemplates(labelTemplates)
	if err != nil {
		return nil, err
	}
	parsedTaints, err := parseTaintTemplates(taintTemplates)
	if err != nil {
		return nil, err
	}

	return &Controller{
		nodeName:       os.Getenv("NODE_NAME"),
		clientset:      clientset,
		labelTemplates: parsedLabels,
		taintTemplates: parsedTaints,
		engines:        []*autodiscovery.Engine{},
		discoveryData:  map[string]map[string]string{},
		scheduler:      scheduler,
	}, nil
}

func parseLabelTemplates(in map[string]string) (map[string]*template.Template, error) {
	out := map[string]*template.Template{}
	for label, tmpl := range in {
		parsed, err := template.New(label).Funcs(sprig.FuncMap()).Option("missingkey=error").Parse(tmpl)
		if err != nil {
			return nil, err
		}
		out[label] = parsed
	}
	return out, nil
}

func parseTaintTemplates(in map[string]config.TaintTemplate) (map[string]*taintTemplate, error) {
	out := map[string]*taintTemplate{}
	for key, t := range in {
		value, err := template.New("taint:" + key + ":value").Funcs(sprig.FuncMap()).Option("missingkey=error").Parse(t.Value)
		if err != nil {
			return nil, err
		}
		effect, err := template.New("taint:" + key + ":effect").Funcs(sprig.FuncMap()).Option("missingkey=error").Parse(t.Effect)
		if err != nil {
			return nil, err
		}
		out[key] = &taintTemplate{value: value, effect: effect}
	}
	return out, nil
}

func (c *Controller) Register(name string, strategy autodiscovery.DiscoveryStrategy) {
	c.engines = append(c.engines, autodiscovery.NewEngine(name, strategy))
}

func (c *Controller) Start() error {
	engineNames := make([]string, 0, len(c.engines))
	for _, engine := range c.engines {
		engineNames = append(engineNames, engine.Name())
	}
	slog.Info("starting controller", "node", c.nodeName, "engines", engineNames)

	dataChannel := make(chan autodiscovery.EnginePayload)
	for _, engine := range c.engines {
		if err := engine.Start(dataChannel); err != nil {
			return err
		}
	}

	if err := c.watchNode(); err != nil {
		return err
	}

	for {
		select {
		case payload := <-dataChannel:
			slog.Debug("received payload from engine", "engine", payload.EngineName, "data", payload.Data)
			c.discoveryData[payload.EngineName] = payload.Data
			c.scheduler.Trigger()
		case <-c.scheduler.C():
			if err := c.reconcileNode(); err != nil {
				slog.Error("reconciliation failed", "error", err)
			}
		}
	}
}

func (c *Controller) watchNode() error {
	watchInterface, err := c.clientset.CoreV1().Nodes().Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	slog.Info("watching node")
	go func() {
		for event := range watchInterface.ResultChan() {
			if event.Type != watch.Modified {
				continue
			}

			node, ok := event.Object.(*corev1.Node)
			if !ok || node.Name != c.nodeName {
				continue
			}

			slog.Debug("received node update, triggering reconciliation")
			c.scheduler.Trigger()
		}
		slog.Warn("node watch channel closed")
	}()

	return nil
}

func sanitizeLabelValue(value string) string {
	sanitized := labelRegexp.ReplaceAllString(value, "_")
	return strings.TrimFunc(sanitized, func(r rune) bool {
		return strings.Contains("_.-", string(r))
	})
}

func (c *Controller) reconcileNode() error {
	slog.Debug("reconciling node")
	node, err := c.clientset.CoreV1().Nodes().Get(context.Background(), c.nodeName, metav1.GetOptions{})
	if err != nil {
		slog.Error("could not get node", "node", c.nodeName, "error", err)
		return nil
	}

	labels := computeLabelPatch(c.labelTemplates, c.discoveryData, node.Labels)
	upsertTaints, deleteTaintKeys := computeTaintOps(c.taintTemplates, c.discoveryData, node.Spec.Taints)

	if len(labels) == 0 && len(upsertTaints) == 0 && len(deleteTaintKeys) == 0 {
		slog.Debug("no changes detected")
		return nil
	}

	patchBytes, err := buildNodeStrategicMergePatch(labels, upsertTaints, deleteTaintKeys)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = c.clientset.CoreV1().Nodes().Patch(context.Background(), node.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("could not patch node %s: %w", c.nodeName, err)
	}

	slog.Info("node patched", "labels", len(labels), "taintsUpserted", len(upsertTaints), "taintsDeleted", len(deleteTaintKeys))

	return nil
}

func computeLabelPatch(
	templates map[string]*template.Template,
	discoveryData map[string]map[string]string,
	nodeLabels map[string]string,
) map[string]any {
	labels := map[string]any{}

	for label, tmpl := range templates {
		value := &bytes.Buffer{}
		if err := tmpl.Execute(value, discoveryData); err != nil {
			slog.Warn("could not render template", "label", label, "error", err)
			continue
		}
		sanitizedValue := sanitizeLabelValue(value.String())

		currentValue, exists := nodeLabels[label]
		switch {
		case sanitizedValue == "" && exists:
			labels[label] = nil
			slog.Info("label removed", "label", label)
		case sanitizedValue != "" && currentValue != sanitizedValue:
			labels[label] = sanitizedValue
			slog.Info("label changed", "label", label, "value", sanitizedValue)
		}
	}

	return labels
}

func buildNodeStrategicMergePatch(labels map[string]any, upsertTaints []corev1.Taint, deleteTaintKeys []string) ([]byte, error) {
	patch := map[string]any{}
	if len(labels) > 0 {
		patch["metadata"] = map[string]any{"labels": labels}
	}

	if len(upsertTaints) > 0 || len(deleteTaintKeys) > 0 {
		taints := make([]any, 0, len(upsertTaints)+len(deleteTaintKeys))
		for _, t := range upsertTaints {
			taints = append(taints, t)
		}

		for _, key := range deleteTaintKeys {
			taints = append(taints, map[string]any{"key": key, "$patch": "delete"})
		}

		patch["spec"] = map[string]any{"taints": taints}
	}

	return json.Marshal(patch)
}

func computeTaintOps(
	templates map[string]*taintTemplate,
	discoveryData map[string]map[string]string,
	currentTaints []corev1.Taint,
) (upsert []corev1.Taint, deleteKeys []string) {
	if len(templates) == 0 {
		return nil, nil
	}

	currentByKey := map[string]corev1.Taint{}
	for _, t := range currentTaints {
		currentByKey[t.Key] = t
	}

	for key, tmpl := range templates {
		effectBuf := &bytes.Buffer{}
		if err := tmpl.effect.Execute(effectBuf, discoveryData); err != nil {
			slog.Warn("could not render effect template", "taint", key, "error", err)
			continue
		}

		effect := corev1.TaintEffect(strings.TrimSpace(effectBuf.String()))
		if effect == "" {
			if _, existed := currentByKey[key]; existed {
				deleteKeys = append(deleteKeys, key)
				slog.Info("taint removed", "key", key)
			}
			continue
		}

		if !isValidTaintEffect(effect) {
			slog.Warn("invalid taint effect", "taint", key, "effect", effect)
			continue
		}

		valueBuf := &bytes.Buffer{}
		if err := tmpl.value.Execute(valueBuf, discoveryData); err != nil {
			slog.Warn("could not render value template", "taint", key, "error", err)
			continue
		}

		taint := corev1.Taint{Key: key, Value: sanitizeLabelValue(valueBuf.String()), Effect: effect}
		if existing, ok := currentByKey[key]; ok && existing.Value == taint.Value && existing.Effect == taint.Effect {
			continue
		}

		upsert = append(upsert, taint)
		slog.Info("taint changed", "key", key, "value", taint.Value, "effect", taint.Effect)
	}

	return upsert, deleteKeys
}

func isValidTaintEffect(effect corev1.TaintEffect) bool {
	switch effect {
	case corev1.TaintEffectNoSchedule, corev1.TaintEffectPreferNoSchedule, corev1.TaintEffectNoExecute:
		return true
	}
	return false
}
