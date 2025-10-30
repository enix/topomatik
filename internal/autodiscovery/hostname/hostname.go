package hostname

import (
	"fmt"
	"os"
	"regexp"

	"github.com/enix/topomatik/internal/autodiscovery/generic/interval"
)

type Engine struct {
	interval.Engine `yaml:",inline"`
	Pattern         *regexp.Regexp `yaml:"pattern"`
	GroupNames      []string       `yaml:"groupNames"` // TODO: ensure that there are no more group names than groups in pattern
}

func (e *Engine) Watch(callback func(map[string]string, error)) {
	e.OnInterval(func() {
		hostname, err := os.Hostname()
		if err != nil {
			fmt.Println("Error getting hostname:", err)
			return
		}

		// matches := e.Pattern.FindStringSubmatch(hostname)
		//
		// if matches != nil {
		// 	zone := matches[1]
		// 	rack := matches[2]
		// 	node := matches[3]
		//
		// 	fmt.Printf("Zone: %s, Rack: %s, Node: %s\n", zone, rack, node)
		// } else {
		// 	fmt.Println("No match")
		// }

		callback(map[string]string{
			"hostname": hostname,
		}, nil)
	})
}
