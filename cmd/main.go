package main

import (
	"flag"
	"fmt"

	"github.com/enix/topomatik/internal/autodiscovery/files"
	"github.com/enix/topomatik/internal/autodiscovery/lldp"
	"github.com/enix/topomatik/internal/config"
	"github.com/enix/topomatik/internal/controller"
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

	ctrl, err := controller.New(k8sClientset, config.LabelTemplates)
	if err != nil {
		panic(err)
	}

	if config.LLDP.Enabled {
		ctrl.Register("lldp", &lldp.LLDPDiscoveryEngine{Config: config.LLDP.Config})
	}

	if len(config.Files) > 0 {
		ctrl.Register("files", &files.FilesDiscoveryEngine{Config: config.Files})
	}

	panic(ctrl.Start(config.MinimumReconciliationInterval))
}
