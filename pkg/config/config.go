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
	TypingSettings struct {
		BaseCharsPerSecond float64 `yaml:"base_chars_per_second"`
		MinDurationMs      int     `yaml:"min_duration_ms"`
		MaxDurationMs      int     `yaml:"max_duration_ms"`
		Variation          float64 `yaml:"variation"`
	} `yaml:"typing"`
	LonelinessSettings struct {
		InactivityThresholdHours  float64 `yaml:"inactivity_threshold_hours"`
		LonelinessThresholdHours  float64 `yaml:"loneliness_threshold_hours"`
		MaxDMAttempts             int     `yaml:"max_dm_attempts"`
		CheckIntervalMinutes      int     `yaml:"check_interval_minutes"`
	} `yaml:"loneliness"`
	AffectionSettings struct {
		MaxAffection        int     `yaml:"max_affection"`
		JealousyThreshold   int     `yaml:"jealousy_threshold_days"`
		JealousyPenalty     int     `yaml:"jealousy_penalty"`
		RandomEventChance   float64 `yaml:"random_event_chance"`
		LateNightStartHour  int     `yaml:"late_night_start_hour"`
		LateNightEndHour    int     `yaml:"late_night_end_hour"`
		MaxStreakDays       int     `yaml:"max_streak_days"`
		StreakBreakPenaltyPerDay int `yaml:"streak_break_penalty_per_day"`
		MaxStreakBreakPenalty    int `yaml:"max_streak_break_penalty"`
	} `yaml:"affection"`
	MessageSettings struct {
		MaxMessageLength     int     `yaml:"max_message_length"`
		ReplyScoreThreshold  float64 `yaml:"reply_score_threshold"`
		CleanupIntervalHours int     `yaml:"cleanup_interval_hours"`
	} `yaml:"messages"`
	StorageSettings struct {
		RecentMessagesLimit     int     `yaml:"recent_messages_limit"`
		RecentMessagesKeep      int     `yaml:"recent_messages_keep"`
		DuplicateThreshold      float64 `yaml:"duplicate_threshold"`
		SimilarityThreshold     float64 `yaml:"similarity_threshold"`
		CleanupProbability      float64 `yaml:"cleanup_probability"`
	} `yaml:"storage"`
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
		// Typing defaults
		config.TypingSettings.BaseCharsPerSecond = 25.0
		config.TypingSettings.MinDurationMs = 800
		config.TypingSettings.MaxDurationMs = 4000
		config.TypingSettings.Variation = 0.3
		// Loneliness defaults
		config.LonelinessSettings.InactivityThresholdHours = 24
		config.LonelinessSettings.LonelinessThresholdHours = 4
		config.LonelinessSettings.MaxDMAttempts = 4
		config.LonelinessSettings.CheckIntervalMinutes = 60
		// Affection defaults
		config.AffectionSettings.MaxAffection = 100000
		config.AffectionSettings.JealousyThreshold = 3
		config.AffectionSettings.JealousyPenalty = -100
		config.AffectionSettings.RandomEventChance = 0.05
		config.AffectionSettings.LateNightStartHour = 23
		config.AffectionSettings.LateNightEndHour = 4
		config.AffectionSettings.MaxStreakDays = 30
		config.AffectionSettings.StreakBreakPenaltyPerDay = 50
		config.AffectionSettings.MaxStreakBreakPenalty = 2500
		// Message defaults
		config.MessageSettings.MaxMessageLength = 280
		config.MessageSettings.ReplyScoreThreshold = 0.6
		config.MessageSettings.CleanupIntervalHours = 1
		// Storage defaults
		config.StorageSettings.RecentMessagesLimit = 20
		config.StorageSettings.RecentMessagesKeep = 15
		config.StorageSettings.DuplicateThreshold = 0.8
		config.StorageSettings.SimilarityThreshold = 0.6
		config.StorageSettings.CleanupProbability = 0.1
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
