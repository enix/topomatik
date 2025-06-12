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

type Controller struct {
	nodeName       string
	clientset      *kubernetes.Clientset
	labelTemplates map[string]*template.Template
	engines        map[string]autodiscovery.Engine
	discoveryData  map[string]map[string]string
}

type EnginePayload struct {
	EngineName string
	Data       map[string]string
}

func New(clientset *kubernetes.Clientset, labelTemplates map[string]string) (*Controller, error) {
	controller := &Controller{
		nodeName:       os.Getenv("NODE_NAME"),
		clientset:      clientset,
		labelTemplates: map[string]*template.Template{},
		engines:        map[string]autodiscovery.Engine{},
		discoveryData:  make(map[string]map[string]string),
	}

	for label, tmpl := range labelTemplates {
		controller.labelTemplates[label] = template.New(label).Funcs(sprig.FuncMap()).Option("missingkey=error")
		if _, err := controller.labelTemplates[label].Parse(tmpl); err != nil {
			return nil, err
		}
	}

	return controller, nil
}

func (c *Controller) Register(name string, engine autodiscovery.Engine) {
	c.engines[name] = engine
}

func (c *Controller) Start() error {
	println("NODE_NAME:", c.nodeName)

	dataChannel := make(chan EnginePayload)

	for name, engine := range c.engines {
		callback := func(data map[string]string) {
			dataChannel <- EnginePayload{
				EngineName: name,
				Data:       data,
			}
		}

		if err := engine.Setup(); err != nil {
			return err
		}

		go engine.Watch(callback)
	}

	c.watchNode()

	for payload := range dataChannel {
		c.discoveryData[payload.EngineName] = payload.Data

		node, err := c.clientset.CoreV1().Nodes().Get(context.Background(), c.nodeName, metav1.GetOptions{})
		if err != nil {
			fmt.Printf("could not get node %s: %s", c.nodeName, err.Error())
			continue
		}

		if err := c.reconcileNode(node); err != nil {
			fmt.Println(err.Error())
		}
	}

	return nil
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
			if err := c.reconcileNode(node); err != nil {
				fmt.Println(err.Error())
			}
		}
	}()

	return nil
}

func (c *Controller) reconcileNode(node *corev1.Node) error {
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

	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
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
