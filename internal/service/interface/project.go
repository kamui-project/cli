package iface

import (
	"context"
	"encoding/json"
	"time"
)

// ProjectStatus represents the status counts for a project or app
type ProjectStatus struct {
	StatusRunning int `json:"status_running"`
	StatusStopped int `json:"status_stopped"`
	StatusError   int `json:"status_error"`
	StatusUnknown int `json:"status_unknown"`
}

// Project represents a Kamui project
type Project struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	PlanType    string     `json:"plan_type"`
	Region      string     `json:"region"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Apps        []App      `json:"apps,omitempty"`
	Databases   []Database `json:"database,omitempty"`
}

// App represents a Kamui application
type App struct {
	ID          string         `json:"id"`
	Name        string         `json:"app_name"`
	DisplayName string         `json:"app_display_name,omitempty"`
	Status      *ProjectStatus `json:"status,omitempty"`
	URL         string         `json:"url,omitempty"`
	AppType     string         `json:"app_type"`
}

// Database represents a Kamui database
type Database struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	SpecType string `json:"spec_type"`
}

func (d *Database) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		DBName         string `json:"db_name"`
		Status         string `json:"status"`
		DatabaseStatus string `json:"database_status"`
		SpecType       string `json:"spec_type"`
		DBSpecType     string `json:"db_spec_type"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	d.ID = raw.ID
	d.Name = firstNonEmpty(raw.Name, raw.DBName)
	d.Status = firstNonEmpty(raw.Status, raw.DatabaseStatus)
	d.SpecType = firstNonEmpty(raw.SpecType, raw.DBSpecType)
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// CreateProjectInput represents the input for creating a project
type CreateProjectInput struct {
	Name        string
	Description string
	PlanType    string
	Region      string
}

// ProjectService defines the interface for project operations
type ProjectService interface {
	// ListProjects returns all projects for the authenticated user
	ListProjects(ctx context.Context) ([]Project, error)

	// GetProject returns a project by ID
	GetProject(ctx context.Context, id string) (*Project, error)

	// CreateProject creates a new project
	CreateProject(ctx context.Context, input *CreateProjectInput) error

	// DeleteProject deletes a project by ID
	DeleteProject(ctx context.Context, id string) error
}
