// Author: zhangweiwei
package ai

import (
	"context"
	"encoding/json"

	"claude-session/internal/model"
	"claude-session/internal/store"
)

const settingsKey = "ai.settings"

func defaultSettings() model.AISettings {
	return model.AISettings{
		Enabled:               true,
		MinMessages:           2,
		MaxChars:              1500,
		IncludeConclusionTail: true,
		AutoEnqueueLimit:      50,
		SummaryBatchSize:      15,
		AutoCategorize:        false,
		ExcludeProjects:       []string{},
	}
}

func LoadSettings(ctx context.Context, st *store.Store) model.AISettings {
	cfg := defaultSettings()
	raw, ok, err := st.GetSetting(ctx, settingsKey)
	if err != nil || !ok || raw == "" {
		return cfg
	}
	if json.Unmarshal([]byte(raw), &cfg) != nil {
		return defaultSettings()
	}
	if cfg.MaxChars <= 0 {
		cfg.MaxChars = 1500
	}
	if cfg.AutoEnqueueLimit <= 0 {
		cfg.AutoEnqueueLimit = 50
	}
	if cfg.SummaryBatchSize <= 0 {
		cfg.SummaryBatchSize = 15
	}
	if cfg.SummaryBatchSize > 20 {
		cfg.SummaryBatchSize = 20
	}
	if cfg.MinMessages < 0 {
		cfg.MinMessages = 2
	}
	if cfg.ExcludeProjects == nil {
		cfg.ExcludeProjects = []string{}
	}
	return cfg
}

func SaveSettings(ctx context.Context, st *store.Store, cfg model.AISettings) error {
	if cfg.MaxChars <= 0 {
		cfg.MaxChars = 1500
	}
	if cfg.AutoEnqueueLimit <= 0 {
		cfg.AutoEnqueueLimit = 50
	}
	if cfg.SummaryBatchSize <= 0 {
		cfg.SummaryBatchSize = 15
	}
	if cfg.SummaryBatchSize > 20 {
		cfg.SummaryBatchSize = 20
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return st.SetSetting(ctx, settingsKey, string(data))
}

func excluded(setting model.AISettings, projectPath string) bool {
	for _, p := range setting.ExcludeProjects {
		if p != "" && p == projectPath {
			return true
		}
	}
	return false
}
