package main

import (
	"flag"

	"github.com/enix/topomatik/internal/autodiscovery/lldp"
	"github.com/enix/topomatik/internal/config"
	"github.com/enix/topomatik/internal/controller"
)

func main() {
	var configPath string

	flag.StringVar(&configPath, "config", "/etc/topomatik/config.yaml", "Path to the configuration file")
	flag.Parse()

	config, err := config.Load(configPath)
	if err != nil {
		panic(err)
	}

	ctrl, err := controller.New(config.AnnotationTemplates)
	if err != nil {
		panic(err)
	}

	if config.LLDP.Enabled {
		ctrl.Register("lldp", &lldp.LLDPDiscoveryService{
			Interface: config.LLDP.Interface,
		})
	}

	panic(ctrl.Start())
}
