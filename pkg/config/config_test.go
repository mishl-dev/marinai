package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Provide a path that definitely doesn't exist
	config, err := LoadConfig("non_existent_config.yml")
	require.NoError(t, err)

	// Verify default values
	assert.Equal(t, 1.0, config.ModelSettings.Temperature)
	assert.Equal(t, 1.0, config.ModelSettings.TopP)
	assert.Equal(t, 0.5, config.Delays.MessageProcessing)
	assert.Equal(t, 7, config.MemorySettings.FactAgingDays)
	assert.Equal(t, 20, config.MemorySettings.FactSummarizationThreshold)
	assert.Equal(t, 24.0, config.MemorySettings.MaintenanceIntervalHours)
}

func TestLoadConfig_ValidFile(t *testing.T) {
	// Create a temporary config file
	content := []byte(`
model_settings:
  temperature: 0.7
  top_p: 0.9
delays:
  message_processing: 1.5
memory:
  fact_aging_days: 14
  fact_summarization_threshold: 50
  maintenance_interval_hours: 12
`)
	tmpfile, err := os.CreateTemp("", "config_test_*.yml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name()) // clean up

	if _, err := tmpfile.Write(content); err != nil {
		tmpfile.Close()
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Load the config
	config, err := LoadConfig(tmpfile.Name())
	require.NoError(t, err)

	// Verify loaded values
	assert.Equal(t, 0.7, config.ModelSettings.Temperature)
	assert.Equal(t, 0.9, config.ModelSettings.TopP)
	assert.Equal(t, 1.5, config.Delays.MessageProcessing)
	assert.Equal(t, 14, config.MemorySettings.FactAgingDays)
	assert.Equal(t, 50, config.MemorySettings.FactSummarizationThreshold)
	assert.Equal(t, 12.0, config.MemorySettings.MaintenanceIntervalHours)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	// Create a temporary file with invalid YAML
	content := []byte(`
model_settings:
  temperature: "not a number"
  broken_yaml: [ unclosed bracket
`)
	tmpfile, err := os.CreateTemp("", "config_invalid_*.yml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(content); err != nil {
		tmpfile.Close()
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Attempt to load the config
	config, err := LoadConfig(tmpfile.Name())

	// Should return an error
	assert.Error(t, err)
	assert.Nil(t, config)
}
