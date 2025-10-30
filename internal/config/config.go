package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/enix/topomatik/internal/autodiscovery/files"
	"github.com/enix/topomatik/internal/autodiscovery/hardware"
	"github.com/enix/topomatik/internal/autodiscovery/hostname"
	"github.com/enix/topomatik/internal/autodiscovery/lldp"
	"github.com/go-playground/validator"
	"gopkg.in/yaml.v2"
)

type Config struct {
	LabelTemplates                map[string]string `yaml:"labelTemplates"`
	MinimumReconciliationInterval time.Duration     `yaml:"minimumReconciliationInterval"`

	LLDP     EngineConfig[lldp.Config]     `yaml:"lldp"`
	Files    files.Config                  `yaml:"files" validate:"dive"`
	Hardware EngineConfig[hardware.Engine] `yaml:"hardware"`
	Hostname EngineConfig[hostname.Engine] `yaml:"hostname"`
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
	validate.RegisterValidation("abs_path_or_url", func(fl validator.FieldLevel) bool {
		v := fl.Field().String()

		info, err := os.Stat(v)
		if err == nil && !info.IsDir() {
			return true
		}

		_, err = url.ParseRequestURI(v)
		if err != nil {
			return false
		}

		return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")
	})

	if err := validate.Struct(config); err != nil {
		if errs, ok := err.(validator.ValidationErrors); ok {
			for _, e := range errs {
				switch e.Tag() {
				case "abs_path_or_url":
					err = fmt.Errorf("%w: \"%s\" must be a path to an existing file or a valid url starting with http(s)://", err, e.Value())
				}
			}
		}
		return nil, err
	}

	if config.MinimumReconciliationInterval == 0 {
		config.MinimumReconciliationInterval = 60
	}

	return &config, nil
}
