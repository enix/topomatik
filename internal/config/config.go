package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	LabelTemplates map[string]string `yaml:"labelTemplates"`

	LLDP LLDP `yaml:"lldp"`
}

type LLDP struct {
	Enabled   bool   `yaml:"enabled"`
	Interface string `yaml:"interface"`
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
