package autodiscovery

type Engine interface {
	Setup() error
	Watch(func(data map[string]string))
}
