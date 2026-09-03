package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// GetSetting returns the setting value and whether it exists.
func (s *Store) GetSetting(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}

// SetSetting upserts a setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value, time.Now().UTC().Format(time.RFC3339))
	return err
}

// GetImportanceConfig returns the persisted importance config, or defaults when unset/malformed.
func (s *Store) GetImportanceConfig() ImportanceConfig {
	cfg := DefaultImportanceConfig()
	raw, ok, err := s.GetSetting("importance_config")
	if err != nil || !ok {
		return cfg
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return DefaultImportanceConfig()
	}
	return cfg
}

// SetImportanceConfig persists the importance config as JSON.
func (s *Store) SetImportanceConfig(cfg ImportanceConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.SetSetting("importance_config", string(data))
}

// GetDecayConfig returns the persisted decay config, or defaults when unset/malformed.
func (s *Store) GetDecayConfig() DecayConfig {
	cfg := DefaultDecayConfig()
	raw, ok, err := s.GetSetting("decay_config")
	if err != nil || !ok {
		return cfg
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return DefaultDecayConfig()
	}
	return cfg
}

// SetDecayConfig persists the decay config as JSON.
func (s *Store) SetDecayConfig(cfg DecayConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.SetSetting("decay_config", string(data))
}
