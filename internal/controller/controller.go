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

type Controller struct {
	nodeName       string
	clientset      *kubernetes.Clientset
	labelTemplates map[string]*template.Template
	engines        []*autodiscovery.Engine
	discoveryData  map[string]map[string]string
	scheduler      ReconciliationScheduler
}

func New(clientset *kubernetes.Clientset, scheduler ReconciliationScheduler, labelTemplates map[string]string) (*Controller, error) {
	controller := &Controller{
		nodeName:       os.Getenv("NODE_NAME"),
		clientset:      clientset,
		labelTemplates: map[string]*template.Template{},
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

func (c *Controller) reconcileNode() error {
	slog.Debug("reconciling node")
	node, err := c.clientset.CoreV1().Nodes().Get(context.Background(), c.nodeName, metav1.GetOptions{})
	if err != nil {
		slog.Error("could not get node", "node", c.nodeName, "error", err)
		return nil
	}

	labels := map[string]string{}
	for label, tmpl := range c.labelTemplates {
		value := &bytes.Buffer{}
		err := tmpl.Execute(value, c.discoveryData)
		if err != nil {
			slog.Warn("could not render template", "label", label, "error", err)
		} else {
			sanitizedValue := labelRegexp.ReplaceAllString(value.String(), "_")
			sanitizedValue = strings.TrimFunc(sanitizedValue, func(r rune) bool {
				return strings.Contains("_.-", string(r))
			})
			if node.Labels[label] != sanitizedValue {
				labels[label] = sanitizedValue
				slog.Info("label changed", "label", label, "value", sanitizedValue)
			}
		}
	}

	if len(labels) == 0 {
		slog.Debug("no label changes detected")
		return nil
	}

	patch := map[string]any{
		"metadata": map[string]any{
			"labels": labels,
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = c.clientset.CoreV1().Nodes().Patch(context.Background(), node.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("could not patch node %s: %w", c.nodeName, err)
	}

	slog.Info("node labels patched", "count", len(labels))

	return nil
}
