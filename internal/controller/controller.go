package controller

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/enix/topomatik/internal/autodiscovery"
)

type Controller struct {
	annotationTemplates map[string]*template.Template
	services            map[string]*autodiscovery.EngineHandler
}

func New(annotationTemplates map[string]string) (_ *Controller, err error) {
	controller := &Controller{
		annotationTemplates: map[string]*template.Template{},
		services:            map[string]*autodiscovery.EngineHandler{},
	}

	for annotation, tmpl := range annotationTemplates {
		controller.annotationTemplates[annotation], err = template.New(annotation).Option("missingkey=error").Parse(tmpl)
		if err != nil {
			return nil, err
		}
	}

	return controller, err
}

func (c *Controller) Register(name string, service autodiscovery.Engine) {
	c.services[name] = autodiscovery.NewServiceHandler(service)
}

func (c *Controller) Start() error {
	update := make(chan struct{})

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

		for annotation, tmpl := range c.annotationTemplates {
			value := &bytes.Buffer{}
			err := tmpl.Execute(value, data)
			if err != nil {
				fmt.Printf("could not render template for %s: %s", annotation, err.Error())
			} else {
				fmt.Printf("%s: %s\n", annotation, value)
			}
		}

		fmt.Println("")
	}

	return nil
}
