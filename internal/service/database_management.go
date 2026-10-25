package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/kamui-project/kamui-cli/internal/api"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

var databaseIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
var databaseNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

// Management resolves every database within an explicitly selected project.
// Authentication, billing, ownership and in-use checks remain server enforced.
func (s *databaseService) managementProject(ctx context.Context, reference string) (*api.Client, *api.DatabaseProject, error) {
	if strings.TrimSpace(reference) == "" {
		return nil, nil, errors.New("--project is required (use kamui projects list)")
	}
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	id := reference
	if !databaseIDPattern.MatchString(reference) {
		projects, err := client.ListDatabaseProjects(ctx)
		if err != nil {
			return nil, nil, err
		}
		id = ""
		for _, project := range projects {
			if project.Name != reference {
				continue
			}
			if id != "" {
				return nil, nil, errors.New("project name is ambiguous; use its ID")
			}
			if !databaseIDPattern.MatchString(project.ID) {
				return nil, nil, errors.New("API returned an invalid project ID")
			}
			id = project.ID
		}
		if id == "" {
			return nil, nil, fmt.Errorf("project %q not found; use kamui projects list", reference)
		}
	}
	project, err := client.GetDatabaseProject(ctx, strings.ToLower(id))
	if err != nil {
		return nil, nil, err
	}
	if !strings.EqualFold(project.ID, id) || !databaseIDPattern.MatchString(project.ID) {
		return nil, nil, errors.New("API returned a different project")
	}
	return client, project, nil
}

func managedDatabase(project *api.DatabaseProject, db api.DatabaseMetadata) iface.DatabaseInfo {
	return iface.DatabaseInfo{
		ID: db.ID, Name: db.Name, Status: db.Status, SpecType: db.SpecType,
		ProjectID: project.ID, ProjectName: project.Name,
	}
}

func (s *databaseService) List(ctx context.Context, projectReference string) ([]iface.DatabaseInfo, error) {
	_, project, err := s.managementProject(ctx, projectReference)
	if err != nil {
		return nil, err
	}
	result := make([]iface.DatabaseInfo, 0, len(project.Databases))
	for _, db := range project.Databases {
		if !databaseIDPattern.MatchString(db.ID) {
			return nil, errors.New("API returned an invalid database ID")
		}
		result = append(result, managedDatabase(project, db))
	}
	return result, nil
}

func (s *databaseService) managementDatabase(ctx context.Context, projectReference, reference string) (*api.Client, *iface.DatabaseInfo, error) {
	if strings.TrimSpace(reference) == "" {
		return nil, nil, errors.New("database name or ID is required")
	}
	client, project, err := s.managementProject(ctx, projectReference)
	if err != nil {
		return nil, nil, err
	}
	var selected *iface.DatabaseInfo
	byID := databaseIDPattern.MatchString(reference)
	for _, db := range project.Databases {
		matches := db.Name == reference
		if byID {
			matches = strings.EqualFold(db.ID, reference)
		}
		if !matches {
			continue
		}
		if selected != nil {
			return nil, nil, errors.New("database name is ambiguous; use its ID")
		}
		if !databaseIDPattern.MatchString(db.ID) {
			return nil, nil, errors.New("API returned an invalid database ID")
		}
		result := managedDatabase(project, db)
		selected = &result
	}
	if selected == nil {
		return nil, nil, fmt.Errorf("database %q not found in project %q", reference, projectReference)
	}
	return client, selected, nil
}

func (s *databaseService) Get(ctx context.Context, projectReference, reference string) (*iface.DatabaseInfo, error) {
	_, db, err := s.managementDatabase(ctx, projectReference, reference)
	return db, err
}

func (s *databaseService) Create(ctx context.Context, input iface.CreateDatabaseInput) (*iface.CreateDatabaseOutput, error) {
	if len(input.Name) == 0 || len(input.Name) > 30 || !databaseNamePattern.MatchString(input.Name) || strings.HasSuffix(input.Name, "_user") || strings.HasSuffix(input.Name, "_external") {
		return nil, errors.New("--name must be 1-30 lowercase letters, digits, underscores or hyphens, start with a letter or underscore, and not end with _user or _external")
	}
	switch input.SpecType {
	case "nano", "small", "medium", "large":
	default:
		return nil, errors.New("--spec must be nano, small, medium, or large")
	}
	switch input.RoutingMode {
	case "shared", "dedicated":
	default:
		return nil, errors.New("--routing-mode must be shared or dedicated")
	}
	client, project, err := s.managementProject(ctx, input.Project)
	if err != nil {
		return nil, err
	}
	result, err := client.CreateDatabase(ctx, api.CreateDatabaseRequest{ProjectID: project.ID, Name: input.Name, SpecType: input.SpecType, RoutingMode: input.RoutingMode})
	if err != nil {
		return nil, err
	}
	if !databaseIDPattern.MatchString(result.DatabaseID) {
		return nil, errors.New("database creation may have been accepted but the API returned no valid database ID; check kamui db list before retrying")
	}
	return &iface.CreateDatabaseOutput{DatabaseID: result.DatabaseID, ProjectID: project.ID, Name: input.Name, SpecType: input.SpecType, RoutingMode: input.RoutingMode, Status: "creation_requested"}, nil
}

func (s *databaseService) Delete(ctx context.Context, projectReference, reference string) error {
	client, db, err := s.managementDatabase(ctx, projectReference, reference)
	if err != nil {
		return err
	}
	return client.DeleteDatabase(ctx, db.ID)
}
