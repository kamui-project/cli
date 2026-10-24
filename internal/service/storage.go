package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/kamui-project/kamui-cli/internal/api"
	"github.com/kamui-project/kamui-cli/internal/config"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
	"github.com/kamui-project/kamui-cli/internal/storage"
)

type storageService struct {
	config   *config.Manager
	auth     iface.AuthService
	projects iface.ProjectService
}

func NewStorageService(config *config.Manager, auth iface.AuthService, projects iface.ProjectService) iface.StorageService {
	return &storageService{config: config, auth: auth, projects: projects}
}
func (s *storageService) api(ctx context.Context) (*api.Client, error) {
	if err := s.auth.EnsureAuthenticated(ctx); err != nil {
		return nil, err
	}
	token, err := s.config.GetAccessToken()
	if err != nil {
		return nil, err
	}
	endpoint, err := s.config.GetAPIURL()
	if err != nil {
		return nil, err
	}
	return api.NewClient(endpoint, token), nil
}
func (s *storageService) project(ctx context.Context, reference string) (string, error) {
	if strings.TrimSpace(reference) == "" {
		return "", errors.New("--project is required")
	}
	if storage.ValidID(reference) {
		return reference, nil
	}
	projects, err := s.projects.ListProjects(ctx)
	if err != nil {
		return "", err
	}
	id := ""
	for _, p := range projects {
		if p.Name == reference {
			if id != "" {
				return "", errors.New("project name is ambiguous; use its ID")
			}
			id = p.ID
		}
	}
	if !storage.ValidID(id) {
		return "", fmt.Errorf("project %q not found; use kamui projects list", reference)
	}
	return id, nil
}
func (s *storageService) List(ctx context.Context, project string) (*storage.Overview, error) {
	id, err := s.project(ctx, project)
	if err != nil {
		return nil, err
	}
	client, err := s.api(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListStorages(ctx, id)
}
func (s *storageService) Create(ctx context.Context, project, name string) (*storage.Item, error) {
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 {
		return nil, errors.New("--name must be 1-80 Unicode characters")
	}
	id, err := s.project(ctx, project)
	if err != nil {
		return nil, err
	}
	client, err := s.api(ctx)
	if err != nil {
		return nil, err
	}
	return client.CreateStorage(ctx, id, name)
}
func (s *storageService) resolve(ctx context.Context, project, reference string) (string, string, error) {
	if reference == "" {
		return "", "", errors.New("storage name or ID is required")
	}
	projectID, err := s.project(ctx, project)
	if err != nil {
		return "", "", err
	}
	client, err := s.api(ctx)
	if err != nil {
		return "", "", err
	}
	overview, err := client.ListStorages(ctx, projectID)
	if err != nil {
		return "", "", err
	}
	id := ""
	for _, item := range overview.Items {
		if item.ID == reference || item.Name == reference {
			if id != "" {
				return "", "", errors.New("storage name is ambiguous; use its ID")
			}
			id = item.ID
		}
	}
	if !storage.ValidID(id) {
		return "", "", fmt.Errorf("storage %q not found in this project", reference)
	}
	return projectID, id, nil
}
func (s *storageService) Delete(ctx context.Context, project, reference string) error {
	projectID, id, err := s.resolve(ctx, project, reference)
	if err != nil {
		return err
	}
	client, err := s.api(ctx)
	if err != nil {
		return err
	}
	return client.DeleteStorage(ctx, projectID, id)
}
func (s *storageService) data(ctx context.Context, project, reference, role string) (*storage.Client, error) {
	projectID, id, err := s.resolve(ctx, project, reference)
	if err != nil {
		return nil, err
	}
	return storage.NewClient(id, role, func(ctx context.Context) (*storage.Permit, error) {
		client, err := s.api(ctx)
		if err != nil {
			return nil, err
		}
		return client.GetStoragePermit(ctx, projectID, id, role)
	}), nil
}
func (s *storageService) ListFiles(ctx context.Context, project, reference, prefix, cursor string, limit int) (*storage.ObjectPage, error) {
	if limit < 1 || limit > 1000 {
		return nil, errors.New("--limit must be between 1 and 1000")
	}
	client, err := s.data(ctx, project, reference, "read_only")
	if err != nil {
		return nil, err
	}
	return client.List(ctx, prefix, cursor, limit)
}
func (s *storageService) Upload(ctx context.Context, project, reference, path, key string) (*storage.Upload, error) {
	if err := storage.ValidateKey(key); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > storage.MaxFileSize {
		return nil, errors.New("upload requires a regular file no larger than 100 MiB")
	}
	client, err := s.data(ctx, project, reference, "read_write")
	if err != nil {
		return nil, err
	}
	return client.UploadFile(ctx, key, file)
}
func (s *storageService) Download(ctx context.Context, project, reference, key, path string, force bool) (int64, error) {
	if err := storage.ValidateKey(key); err != nil {
		return 0, err
	}
	if path == "" || path == "-" {
		return 0, errors.New("provide an explicit destination file path")
	}
	if !force {
		if _, err := os.Lstat(path); err == nil {
			return 0, errors.New("destination exists; use --force to replace it")
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".kamui-download-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	client, err := s.data(ctx, project, reference, "read_only")
	if err != nil {
		return 0, err
	}
	if err = client.Download(ctx, key, file); err != nil {
		return 0, err
	}
	if err = file.Sync(); err != nil {
		return 0, err
	}
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if err = file.Close(); err != nil {
		return 0, err
	}
	if force {
		err = os.Rename(file.Name(), path)
	} else {
		err = os.Link(file.Name(), path)
	}
	if err != nil {
		return 0, fmt.Errorf("could not publish downloaded file: %w", err)
	}
	return info.Size(), nil
}
func (s *storageService) DeleteFile(ctx context.Context, project, reference, key string) error {
	if err := storage.ValidateKey(key); err != nil {
		return err
	}
	client, err := s.data(ctx, project, reference, "read_write")
	if err != nil {
		return err
	}
	return client.Delete(ctx, key)
}
