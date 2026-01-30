package autodiscovery

import (
	"context"
	"log/slog"

	"github.com/enix/topomatik/internal/logging"
)

type DiscoveryStrategy interface {
	Setup(ctx context.Context) error
	Watch(ctx context.Context, callback func(data map[string]string, err error))
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

func (e *Engine) Name() string {
	return e.name
}

func (e *Engine) Start(dataChannel chan<- EnginePayload) error {
	logger := slog.Default().With("engine", e.name)
	ctx := logging.NewContext(context.Background(), logger)

	if err := e.strategy.Setup(ctx); err != nil {
		return err
	}

	go e.strategy.Watch(ctx, func(data map[string]string, err error) {
		if err != nil {
			logger.Error("engine encountered an error", "error", err)
			return
		}
		dataChannel <- EnginePayload{
			EngineName: e.name,
			Data:       data,
		}
	})

	return nil
}
