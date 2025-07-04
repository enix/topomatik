package config

import (
	"os"

	"github.com/enix/topomatik/internal/autodiscovery/lldp"
	"gopkg.in/yaml.v2"
)

type Config struct {
	LabelTemplates map[string]string `yaml:"labelTemplates"`

	LLDP EngineConfig[lldp.Config] `yaml:"lldp"`
}

type EngineConfig[T any] struct {
	Config  T    `yaml:",inline"`
	Enabled bool `yaml:"enabled"`
}

func Load(path string) (*Config, error) {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err = yaml.Unmarshal(yamlFile, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
