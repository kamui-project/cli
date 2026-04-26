package service

import (
	"context"
	"fmt"

	"github.com/kamui-project/kamui-cli/internal/api"
	"github.com/kamui-project/kamui-cli/internal/config"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

// tokensService implements iface.TokensService
type tokensService struct {
	configManager *config.Manager
	authService   iface.AuthService
}

// NewTokensService creates a new tokens service.
func NewTokensService(configManager *config.Manager, authService iface.AuthService) iface.TokensService {
	return &tokensService{
		configManager: configManager,
		authService:   authService,
	}
}

func (s *tokensService) getAPIClient(ctx context.Context) (*api.Client, error) {
	if err := s.authService.EnsureAuthenticated(ctx); err != nil {
		return nil, err
	}
	token, err := s.configManager.GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}
	apiURL, err := s.configManager.GetAPIURL()
	if err != nil {
		return nil, fmt.Errorf("failed to get API URL: %w", err)
	}
	return api.NewClient(apiURL, token), nil
}

func (s *tokensService) Create(ctx context.Context, name string, expiresInDays int) (string, string, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return "", "", err
	}
	return client.CreatePAT(ctx, name, expiresInDays)
}

func (s *tokensService) List(ctx context.Context, includeOAuth bool) ([]iface.PATInfo, error) {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	pats, err := client.ListPATs(ctx, includeOAuth)
	if err != nil {
		return nil, err
	}
	out := make([]iface.PATInfo, len(pats))
	for i, p := range pats {
		out[i] = iface.PATInfo{
			ID:         p.ID,
			Name:       p.Name,
			ExpiresAt:  p.ExpiresAt,
			CreatedAt:  p.CreatedAt,
			LastUsedAt: p.LastUsedAt,
		}
	}
	return out, nil
}

func (s *tokensService) Delete(ctx context.Context, id string) error {
	client, err := s.getAPIClient(ctx)
	if err != nil {
		return err
	}
	return client.DeletePAT(ctx, id)
}
