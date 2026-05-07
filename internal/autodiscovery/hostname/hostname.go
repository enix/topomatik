package hostname

import (
	"context"
	"os"
	"regexp"
	"time"
)

type Config struct {
	Interval time.Duration  `yaml:"interval" validate:"required"`
	Pattern  *regexp.Regexp `yaml:"pattern"`
}

type HostnameDiscoveryEngine struct {
	Config
}

func (h *HostnameDiscoveryEngine) Setup(_ context.Context) (err error) {
	return
}

func (h *HostnameDiscoveryEngine) Watch(_ context.Context, callback func(data map[string]string, err error)) {
	ticker := time.NewTicker(h.Interval)

	for {
		hostname, err := os.Hostname()
		if err != nil {
			callback(nil, err)
			continue
		}

		callback(extractHostnameData(hostname, h.Pattern), nil)
		<-ticker.C
	}
}

func extractHostnameData(hostname string, pattern *regexp.Regexp) map[string]string {
	data := map[string]string{"value": hostname}
	if pattern == nil {
		return data
	}
	submatches := pattern.FindStringSubmatch(hostname)
	if submatches == nil {
		return data
	}
	for _, name := range pattern.SubexpNames() {
		if name == "" {
			continue
		}
		data[name] = submatches[pattern.SubexpIndex(name)]
	}
	return data
}
