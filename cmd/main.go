package main

import (
	"flag"
	"log/slog"

	"github.com/enix/topomatik/internal/autodiscovery/files"
	"github.com/enix/topomatik/internal/autodiscovery/hardware"
	"github.com/enix/topomatik/internal/autodiscovery/hostname"
	"github.com/enix/topomatik/internal/autodiscovery/lldp"
	"github.com/enix/topomatik/internal/autodiscovery/network"
	"github.com/enix/topomatik/internal/config"
	"github.com/enix/topomatik/internal/controller"
	"github.com/enix/topomatik/internal/logging"
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
		logFormat      logging.Format
		logLevel       logging.Level
	)

	if Version == "dev" {
		logFormat = logging.FormatText
		logLevel.Level = slog.LevelDebug
	} else {
		logFormat = logging.FormatJSON
		logLevel.Level = slog.LevelInfo
	}

	flag.StringVar(&configPath, "config", "/etc/topomatik/config.yaml", "Path to the configuration file")
	flag.StringVar(&kubeconfigPath, "kubeconfig", "", "Path to a kubeconfig file.")
	flag.Var(&logFormat, "log-format", "Log output format: \"json\" or \"text\"")
	flag.Var(&logLevel, "log-level", "Log level: \"debug\", \"info\", \"warn\", or \"error\"")
	flag.Parse()

	logging.Setup(logFormat, logLevel)

	config, err := config.Load(configPath)
	if err != nil {
		panic(err)
	}

	var kubeconfig *rest.Config

	if kubeconfigPath == "" {
		slog.Info("using in-cluster configuration")
		kubeconfig, err = rest.InClusterConfig()
	} else {
		slog.Info("using kubeconfig", "path", kubeconfigPath)
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
		ctrl.Register("lldp", &lldp.LLDPDiscoveryEngine{Config: config.LLDP.Config})
	}

	if len(config.Files) > 0 {
		ctrl.Register("files", &files.FilesDiscoveryEngine{Config: config.Files})
	}

	if config.Hardware.Enabled {
		ctrl.Register("hardware", &hardware.HardwareDiscoveryEngine{Config: config.Hardware.Config})
	}

	if config.Hostname.Enabled {
		ctrl.Register("hostname", &hostname.HostnameDiscoveryEngine{Config: config.Hostname.Config})
	}

	if config.Network.Enabled {
		ctrl.Register("network", &network.NetworkDiscoveryEngine{Config: config.Network.Config})
	}

	panic(ctrl.Start())
}
