package interval

import (
	"time"

	"github.com/enix/topomatik/internal/autodiscovery"
)

type Setuper interface {
	Setup() error
}

type Watcher interface {
	Watch(callback func(data map[string]string, err error))
}

type GenericEngine struct {
	Setuper
	Watcher // FIXME: what happens if this is nil?
}

func (g *GenericEngine) Setup() error {
	if g.Setuper == nil {
		return nil
	}
	return g.Setuper.Setup()
}

// =============================

type IntervalReader interface {
	Setuper
	Update(callback func(data map[string]string, err error))
}

func New[T any](config *Config[T], strategy IntervalReader) autodiscovery.DiscoveryStrategy {
	return &GenericEngine{
		Setuper: strategy,
		Watcher: GenericIntervalDiscoveryEngine{
			Interval: config.Interval,
			Update:   strategy.Update,
		},
	}
}

type Config[T any] struct {
	Interval time.Duration `yaml:"interval"`
	Config   T             `yaml:",inline"`
}

type GenericIntervalDiscoveryEngine struct {
	Interval time.Duration
	Update   func(callback func(data map[string]string, err error))
}

func (f GenericIntervalDiscoveryEngine) Watch(callback func(data map[string]string, err error)) {
	ticker := time.NewTicker(f.Interval)
	for {
		f.Update(callback)
		<-ticker.C
	}
}
