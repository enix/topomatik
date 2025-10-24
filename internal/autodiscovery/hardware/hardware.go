package hardware

import (
	"time"

	"github.com/jaypipes/ghw"
)

type Config struct {
	Interval time.Duration `yaml:"interval"`
}

type HardwareDiscoveryEngine struct {
	Config
}

func (f *HardwareDiscoveryEngine) Setup() (err error) {
	return
}

func (f *HardwareDiscoveryEngine) Watch(callback func(data map[string]string, err error)) {
	ticker := time.NewTicker(f.Interval)

	for {
		chassis, err := ghw.Chassis()
		if err != nil {
			callback(nil, err)
			return
		}

		callback(map[string]string{
			"chassis_serial":    chassis.SerialNumber,
			"chassis_asset_tag": chassis.AssetTag,
		}, nil)

		<-ticker.C
	}
}
