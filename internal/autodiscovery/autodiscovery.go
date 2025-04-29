package autodiscovery

type Engine interface {
	Watch() (chan map[string]string, error)
}

type EngineHandler struct {
	Data map[string]string

	dataChannel chan map[string]string
	service     Engine
}

func NewServiceHandler(service Engine) *EngineHandler {
	return &EngineHandler{
		service: service,
	}
}

func (h *EngineHandler) Start() (err error) {
	h.dataChannel, err = h.service.Watch()
	return err
}

func (h *EngineHandler) KeepUpdated(update chan struct{}) {
	for data := range h.dataChannel {
		h.Data = data
		update <- struct{}{}
	}
}
