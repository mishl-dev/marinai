package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ModelSettings struct {
		Temperature float64 `yaml:"temperature"`
		TopP        float64 `yaml:"top_p"`
	} `yaml:"model_settings"`
	Delays struct {
		MessageProcessing float64 `yaml:"message_processing"`
	} `yaml:"delays"`
	MemorySettings struct {
		FactAgingDays              int     `yaml:"fact_aging_days"`
		FactSummarizationThreshold int     `yaml:"fact_summarization_threshold"`
		MaintenanceIntervalHours   float64 `yaml:"maintenance_interval_hours"`
	} `yaml:"memory"`
}

func LoadConfig(path string) (*Config, error) {
	config := &Config{}

	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		// Set default values
		config.ModelSettings.Temperature = 1
		config.ModelSettings.TopP = 1
		config.Delays.MessageProcessing = 0.5
		config.MemorySettings.FactAgingDays = 7
		config.MemorySettings.FactSummarizationThreshold = 20
		config.MemorySettings.MaintenanceIntervalHours = 24
		return config, nil
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(file, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}
