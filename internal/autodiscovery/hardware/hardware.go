package hardware

import (
	"context"
	"time"

	"github.com/jaypipes/ghw"
)

type Config struct {
	Interval time.Duration `yaml:"interval" validate:"required"`
}

type HardwareDiscoveryEngine struct {
	Config
}

func (h *HardwareDiscoveryEngine) Setup(_ context.Context) (err error) {
	return
}

func (h *HardwareDiscoveryEngine) Watch(_ context.Context, callback func(data map[string]string, err error)) {
	ticker := time.NewTicker(h.Interval)

	for {
		chassis, err := ghw.Chassis()
		if err != nil {
			callback(nil, err)
			continue
		}

		callback(map[string]string{
			"chassis_serial":    chassis.SerialNumber,
			"chassis_asset_tag": chassis.AssetTag,
		}, nil)

		<-ticker.C
	}
}
