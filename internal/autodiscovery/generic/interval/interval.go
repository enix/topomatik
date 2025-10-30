package interval

import (
	"time"
)

type Config struct {
	Interval time.Duration `yaml:"interval"`
}

type Engine struct {
	Config
}

func (g *Engine) Setup() error {
	return nil
}

func (g *Engine) OnInterval(handler func()) {
	ticker := time.NewTicker(g.Interval)
	for {
		handler()
		<-ticker.C
	}
}
