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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type Controller struct {
	clientset      *kubernetes.Clientset
	labelTemplates map[string]*template.Template
	engines        map[string]autodiscovery.Engine
}

type EnginePayload struct {
	EngineName string
	Data       map[string]string
}

func New(clientset *kubernetes.Clientset, labelTemplates map[string]string) (*Controller, error) {
	controller := &Controller{
		clientset:      clientset,
		labelTemplates: map[string]*template.Template{},
		engines:        map[string]autodiscovery.Engine{},
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
	nodeName := os.Getenv("NODE_NAME")
	println("NODE_NAME:", nodeName)

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

	for payload := range dataChannel {
		data := map[string]map[string]string{}
		data[payload.EngineName] = payload.Data

		node, err := c.clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		if err != nil {
			fmt.Printf("could not get node %s: %s", nodeName, err.Error())
			continue
		}

		labels := map[string]string{}
		for label, tmpl := range c.labelTemplates {
			value := &bytes.Buffer{}
			err := tmpl.Execute(value, data)
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
			continue
		}

		patch := map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": labels,
			},
		}
		patchBytes, err := json.Marshal(patch)
		if err != nil {
			fmt.Printf("failed to marshal patch: %s\n", err)
			continue
		}

		_, err = c.clientset.CoreV1().Nodes().Patch(context.Background(), node.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			fmt.Printf("could not update node %s: %s\n", nodeName, err.Error())
			continue
		}
	}

	return nil
}
