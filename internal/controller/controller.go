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
	clientset      *kubernetes.Clientset
	labelTemplates map[string]*template.Template
	taintTemplates map[string]*taintTemplate
	engines        []*autodiscovery.Engine
	discoveryData  map[string]map[string]string
	scheduler      ReconciliationScheduler
}

func New(clientset *kubernetes.Clientset, scheduler ReconciliationScheduler, labelTemplates map[string]string, taintTemplates map[string]config.TaintTemplate) (*Controller, error) {
	controller := &Controller{
		nodeName:       os.Getenv("NODE_NAME"),
		clientset:      clientset,
		labelTemplates: map[string]*template.Template{},
		taintTemplates: map[string]*taintTemplate{},
		engines:        []*autodiscovery.Engine{},
		discoveryData:  map[string]map[string]string{},
		scheduler:      scheduler,
	}

	for label, tmpl := range labelTemplates {
		controller.labelTemplates[label] = template.New(label).Funcs(sprig.FuncMap()).Option("missingkey=error")
		if _, err := controller.labelTemplates[label].Parse(tmpl); err != nil {
			return nil, err
		}
	}

	for key, t := range taintTemplates {
		value := template.New("taint:" + key + ":value").Funcs(sprig.FuncMap()).Option("missingkey=error")
		if _, err := value.Parse(t.Value); err != nil {
			return nil, err
		}
		effect := template.New("taint:" + key + ":effect").Funcs(sprig.FuncMap()).Option("missingkey=error")
		if _, err := effect.Parse(t.Effect); err != nil {
			return nil, err
		}
		controller.taintTemplates[key] = &taintTemplate{
			value:  value,
			effect: effect,
		}
	}

	return controller, nil
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

	labels := map[string]any{}
	for label, tmpl := range c.labelTemplates {
		value := &bytes.Buffer{}
		err := tmpl.Execute(value, c.discoveryData)
		if err != nil {
			slog.Warn("could not render template", "label", label, "error", err)
			continue
		}
		sanitizedValue := sanitizeLabelValue(value.String())

		currentValue, exists := node.Labels[label]
		switch {
		case sanitizedValue == "" && exists:
			labels[label] = nil
			slog.Info("label removed", "label", label)
		case sanitizedValue != "" && currentValue != sanitizedValue:
			labels[label] = sanitizedValue
			slog.Info("label changed", "label", label, "value", sanitizedValue)
		}
	}

	desiredTaints, taintsChanged := c.computeDesiredTaints(node)

	if len(labels) == 0 && taintsChanged == 0 {
		slog.Debug("no changes detected")
		return nil
	}

	patch := map[string]any{}
	if len(labels) > 0 {
		patch["metadata"] = map[string]any{"labels": labels}
	}
	if taintsChanged > 0 {
		patch["spec"] = map[string]any{"taints": desiredTaints}
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = c.clientset.CoreV1().Nodes().Patch(context.Background(), node.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("could not patch node %s: %w", c.nodeName, err)
	}

	slog.Info("node patched", "labels", len(labels), "taints", taintsChanged)

	return nil
}

func (c *Controller) computeDesiredTaints(node *corev1.Node) ([]corev1.Taint, int) {
	if len(c.taintTemplates) == 0 {
		return nil, 0
	}

	currentByKey := map[string]corev1.Taint{}
	for _, t := range node.Spec.Taints {
		currentByKey[t.Key] = t
	}

	desired := make([]corev1.Taint, 0, len(node.Spec.Taints))
	for _, t := range node.Spec.Taints {
		if _, managed := c.taintTemplates[t.Key]; !managed {
			desired = append(desired, t)
		}
	}

	preserveExisting := func(key string) {
		if existing, ok := currentByKey[key]; ok {
			desired = append(desired, existing)
		}
	}

	changed := 0
	for key, tmpl := range c.taintTemplates {
		effectBuf := &bytes.Buffer{}
		if err := tmpl.effect.Execute(effectBuf, c.discoveryData); err != nil {
			slog.Warn("could not render effect template", "taint", key, "error", err)
			preserveExisting(key)
			continue
		}

		effect := corev1.TaintEffect(strings.TrimSpace(effectBuf.String()))
		if effect == "" {
			if _, existed := currentByKey[key]; existed {
				changed++
				slog.Info("taint removed", "key", key)
			}
			continue
		}

		if !isValidTaintEffect(effect) {
			slog.Warn("invalid taint effect", "taint", key, "effect", effect)
			preserveExisting(key)
			continue
		}

		valueBuf := &bytes.Buffer{}
		if err := tmpl.value.Execute(valueBuf, c.discoveryData); err != nil {
			slog.Warn("could not render value template", "taint", key, "error", err)
			preserveExisting(key)
			continue
		}

		taint := corev1.Taint{Key: key, Value: sanitizeLabelValue(valueBuf.String()), Effect: effect}
		desired = append(desired, taint)

		existing, ok := currentByKey[key]
		if !ok || existing.Value != taint.Value || existing.Effect != taint.Effect {
			changed++
			slog.Info("taint changed", "key", key, "value", taint.Value, "effect", taint.Effect)
		}
	}

	return desired, changed
}

func isValidTaintEffect(effect corev1.TaintEffect) bool {
	switch effect {
	case corev1.TaintEffectNoSchedule, corev1.TaintEffectPreferNoSchedule, corev1.TaintEffectNoExecute:
		return true
	}
	return false
}
