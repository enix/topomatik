package hostname

import (
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

func (h *HostnameDiscoveryEngine) Setup() (err error) {
	return
}

func (h *HostnameDiscoveryEngine) Watch(callback func(data map[string]string, err error)) {
	ticker := time.NewTicker(h.Interval)

	for {
		hostname, err := os.Hostname()
		if err != nil {
			callback(nil, err)
			continue
		}

		data := map[string]string{
			"value": hostname,
		}

		if h.Pattern != nil {
			if submatches := h.Pattern.FindStringSubmatch(hostname); submatches != nil {
				for _, subexpName := range h.Pattern.SubexpNames() {
					if subexpName != "" {
						data[subexpName] = submatches[h.Pattern.SubexpIndex(subexpName)]
					}
				}
			}
		}

		callback(data, nil)
		<-ticker.C
	}
}
