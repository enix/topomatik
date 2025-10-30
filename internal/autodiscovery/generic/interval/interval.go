package interval

import (
	"time"
)

type Handler interface {
	Update(callback func(data map[string]string, err error))
}

type Engine struct {
	Interval time.Duration `yaml:"interval"`
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
