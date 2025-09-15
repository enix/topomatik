package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/enix/topomatik/internal/autodiscovery"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

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
	println("NODE_NAME:", c.nodeName)

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
			c.discoveryData[payload.EngineName] = payload.Data
			c.scheduler.Trigger()
		case <-c.scheduler.C():
			if err := c.reconcileNode(); err != nil {
				fmt.Println(err.Error())
			}
		}
	}
}

func (c *Controller) watchNode() error {
	watchInterface, err := c.clientset.CoreV1().Nodes().Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	go func() {
		for event := range watchInterface.ResultChan() {
			if event.Type != watch.Modified {
				continue
			}

			node, ok := event.Object.(*corev1.Node)
			if !ok || node.Name != c.nodeName {
				continue
			}

			fmt.Println("received a node update, triggering reconciliation")
			c.scheduler.Trigger()
		}
	}()

	return nil
}

func (c *Controller) reconcileNode() error {
	node, err := c.clientset.CoreV1().Nodes().Get(context.Background(), c.nodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("could not get node %s: %s", c.nodeName, err.Error())
		return nil
	}

	labels := map[string]string{}
	for label, tmpl := range c.labelTemplates {
		value := &bytes.Buffer{}
		err := tmpl.Execute(value, c.discoveryData)
		if err != nil {
			fmt.Printf("could not render template for %s: %s\n", label, err.Error())
		} else {
			if node.Labels[label] != value.String() {
				labels[label] = value.String()
				fmt.Printf("%s: %s\n", label, value)
			}
		}
	}

	if len(labels) == 0 {
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

	return nil
}
