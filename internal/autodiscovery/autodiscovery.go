package autodiscovery

import "fmt"

type DiscoveryStrategy interface {
	Setup() error
	Watch(func(data map[string]string, err error))
}

type Engine struct {
	strategy DiscoveryStrategy
	name     string
}

type EnginePayload struct {
	EngineName string
	Data       map[string]string
}

func NewEngine(name string, strategy DiscoveryStrategy) *Engine {
	return &Engine{
		strategy: strategy,
		name:     name,
	}
}

func (e *Engine) Start(dataChannel chan<- EnginePayload) error {
	if err := e.strategy.Setup(); err != nil {
		return err
	}

	go e.strategy.Watch(func(data map[string]string, err error) {
		if err != nil {
			fmt.Printf("%s engine encountered an error: %s\n", e.name, err.Error())
			return
		}
		dataChannel <- EnginePayload{
			EngineName: e.name,
			Data:       data,
		}
	})

	return nil
}
