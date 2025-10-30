package main

import (
	"flag"
	"fmt"

	"github.com/enix/topomatik/internal/autodiscovery/files"
	"github.com/enix/topomatik/internal/autodiscovery/lldp"
	"github.com/enix/topomatik/internal/config"
	"github.com/enix/topomatik/internal/controller"
	"github.com/enix/topomatik/internal/schedulers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var Version = "dev"

func main() {
	var (
		configPath     string
		kubeconfigPath string
	)

	flag.StringVar(&configPath, "config", "/etc/topomatik/config.yaml", "Path to the configuration file")
	flag.StringVar(&kubeconfigPath, "kubeconfig", "", "Path to a kubeconfig file.")
	flag.Parse()

	config, err := config.Load(configPath)
	if err != nil {
		panic(err)
	}

	var kubeconfig *rest.Config

	if kubeconfigPath == "" {
		fmt.Println("using in-cluster configuration")
		kubeconfig, err = rest.InClusterConfig()
	} else {
		fmt.Println("using configuration from file: " + kubeconfigPath)
		kubeconfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}
	if err != nil {
		panic(err)
	}

	k8sClientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		panic(err)
	}

	scheduler := schedulers.NewSometimesWithDebounceChannel(config.MinimumReconciliationInterval)

	ctrl, err := controller.New(k8sClientset, scheduler, config.LabelTemplates)
	if err != nil {
		panic(err)
	}

	if config.LLDP.Enabled {
		// TODO: update this engine to remove Config struct in favor of merged config and engine
		ctrl.Register("lldp", &lldp.LLDPDiscoveryEngine{Config: config.LLDP.Config})
	}

	if len(config.Files) > 0 {
		// TODO: update this engine to remove Config struct in favor of merged config and engine
		ctrl.Register("files", &files.FilesDiscoveryEngine{Config: config.Files})
	}

	// TODO: ctrl.RegisterMany(map[string]discoveryengine{"hardware": config.Hardware.Config})
	// ou mieux: config.Engines and iterate over keys in struct to get the name
	if config.Hardware.Enabled {
		ctrl.Register("hardware", &config.Hardware.Config)
	}

	if config.Hostname.Enabled {
		ctrl.Register("hostname", &config.Hostname.Config)
	}

	panic(ctrl.Start())
}
