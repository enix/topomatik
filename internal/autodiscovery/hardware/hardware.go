package hardware

import (
	"github.com/enix/topomatik/internal/autodiscovery"
	"github.com/enix/topomatik/internal/autodiscovery/generic/interval"
	"github.com/jaypipes/ghw"
)

type Config struct{}

type HardwareDiscoveryEngine struct {
	Config
}

func New(config *interval.Config[Config]) autodiscovery.DiscoveryStrategy {
	return interval.New(config, &HardwareDiscoveryEngine{Config: config.Config})
}

func (f *HardwareDiscoveryEngine) Setup() error {
	return nil
}

func (f *HardwareDiscoveryEngine) Update(callback func(data map[string]string, err error)) {
	chassis, err := ghw.Chassis()
	if err != nil {
		callback(nil, err)
		return
	}

	callback(map[string]string{
		"chassis_serial":    chassis.SerialNumber,
		"chassis_asset_tag": chassis.AssetTag,
	}, nil)
}
