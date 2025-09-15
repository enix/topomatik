package config

import (
	"os"
	"time"

	"github.com/enix/topomatik/internal/autodiscovery/files"
	"github.com/enix/topomatik/internal/autodiscovery/lldp"
	"github.com/go-playground/validator"
	"gopkg.in/yaml.v2"
)

type Config struct {
	LabelTemplates                map[string]string `yaml:"labelTemplates"`
	MinimumReconciliationInterval time.Duration     `yaml:"minimumReconciliationInterval"`

	LLDP  EngineConfig[lldp.Config] `yaml:"lldp"`
	Files files.Config              `yaml:"files" validate:"dive"`
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

	validate := validator.New()
	if err := validate.Struct(config); err != nil {
		return nil, err
	}

	if config.MinimumReconciliationInterval == 0 {
		config.MinimumReconciliationInterval = 60
	}

	return &config, nil
}
