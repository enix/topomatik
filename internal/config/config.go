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
	"github.com/enix/topomatik/internal/autodiscovery/network"
	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v2"
)

type Config struct {
	LabelTemplates                map[string]string `yaml:"labelTemplates"`
	MinimumReconciliationInterval time.Duration     `yaml:"minimumReconciliationInterval"`

	LLDP     EngineConfig[lldp.Config]     `yaml:"lldp"`
	Files    files.Config                  `yaml:"files" validate:"dive"`
	Hardware EngineConfig[hardware.Config] `yaml:"hardware"`
	Hostname EngineConfig[hostname.Config] `yaml:"hostname"`
	Network  EngineConfig[network.Config]  `yaml:"network"`
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
	err = validate.RegisterValidation("abs_path_or_url", func(fl validator.FieldLevel) bool {
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
	if err != nil {
		return nil, err
	}

	validate.RegisterStructValidation(func(sl validator.StructLevel) {
		file := sl.Current().Interface().(files.File)
		info, err := os.Stat(file.Path)
		if err == nil && !info.IsDir() {
			return
		}
		if file.Interval == 0 {
			sl.ReportError(file.Interval, "Interval", "Interval", "required_for_remote_files", "")
		}
	}, files.File{})

	ignoredEngines := []string{}
	if !config.Hardware.Enabled {
		ignoredEngines = append(ignoredEngines, "Hardware")
	}
	if !config.Hostname.Enabled {
		ignoredEngines = append(ignoredEngines, "Hostname")
	}
	if !config.Network.Enabled {
		ignoredEngines = append(ignoredEngines, "Network")
	}

	if err := validate.StructExcept(config, ignoredEngines...); err != nil {
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
