package controller

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/enix/topomatik/internal/autodiscovery"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Controller struct {
	clientset           *kubernetes.Clientset
	annotationTemplates map[string]*template.Template
	services            map[string]*autodiscovery.EngineHandler
}

func New(clientset *kubernetes.Clientset, annotationTemplates map[string]string) (*Controller, error) {
	controller := &Controller{
		clientset:           clientset,
		annotationTemplates: map[string]*template.Template{},
		services:            map[string]*autodiscovery.EngineHandler{},
	}

	for annotation, tmpl := range annotationTemplates {
		controller.annotationTemplates[annotation] = template.New(annotation).Funcs(sprig.FuncMap()).Option("missingkey=error")
		if _, err := controller.annotationTemplates[annotation].Parse(tmpl); err != nil {
			return nil, err
		}
	}

	return controller, nil
}

func (c *Controller) Register(name string, service autodiscovery.Engine) {
	c.services[name] = autodiscovery.NewServiceHandler(service)
}

func (c *Controller) Start() error {
	update := make(chan struct{})
	nodeName := os.Getenv("NODE_NAME")
	println("NODE_NAME:", nodeName)

	for _, service := range c.services {
		if err := service.Start(); err != nil {
			return err
		}

		go service.KeepUpdated(update)
	}

	for range update {
		data := map[string]map[string]string{}
		for name, service := range c.services {
			data[name] = service.Data
		}

		node, err := c.clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
		if err != nil {
			fmt.Printf("could not get node %s: %s", nodeName, err.Error())
			continue
		}

		for annotation, tmpl := range c.annotationTemplates {
			value := &bytes.Buffer{}
			err := tmpl.Execute(value, data)
			if err != nil {
				fmt.Printf("could not render template for %s: %s\n", annotation, err.Error())
			} else {
				node.Annotations[annotation] = value.String()
				fmt.Printf("%s: %s\n", annotation, value)
			}
		}

		_, err = c.clientset.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
		if err != nil {
			fmt.Printf("could not update node %s: %s\n", nodeName, err.Error())
			continue
		}

		fmt.Println("")
	}

	return nil
}
