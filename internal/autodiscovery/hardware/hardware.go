package hardware

import (
	"github.com/enix/topomatik/internal/autodiscovery/generic/interval"
	"github.com/jaypipes/ghw"
)

type Engine struct {
	interval.Engine `yaml:",inline"`
}

func (e *Engine) Watch(callback func(data map[string]string, err error)) {
	e.OnInterval(func() {
		chassis, err := ghw.Chassis()
		if err != nil {
			callback(nil, err)
			return
		}

		callback(map[string]string{
			"chassis_serial":    chassis.SerialNumber,
			"chassis_asset_tag": chassis.AssetTag,
		}, nil)
	})
}
