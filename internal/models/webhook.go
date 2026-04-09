package models

import (
	"time"
)

// Webhook represents a Discord webhook configuration
type Webhook struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	Enabled    bool      `json:"enabled"`
	Events     []string  `json:"events"`
	CreatedAt  time.Time `json:"created_at"`
	LastTested time.Time `json:"last_tested,omitempty"`
	TestStatus string    `json:"test_status,omitempty"` // "success", "failed", "pending", ""
	TestError  string    `json:"test_error,omitempty"`
}

// WebhookConfig holds all webhook configurations
type WebhookConfig struct {
	Webhooks []Webhook `json:"webhooks"`
}

// AvailableEvents returns the list of all events that can be selected
func AvailableEvents() []string {
	return []string{
		"work_block_created",
		"work_block_approved",
		"work_block_rejected",
		"work_block_completed",
		"issue_created",
		"issue_status_changed",
		"run_started",
		"run_completed",
		"agent_created",
	}
}

// HasEvent checks if a webhook is configured for a given event
func (w *Webhook) HasEvent(event string) bool {
	for _, e := range w.Events {
		if e == event {
			return true
		}
	}
	return false
}
