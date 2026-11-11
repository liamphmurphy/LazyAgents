package repositories

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type OwnerPreset struct {
	Login        string `json:"login"`
	Organization bool   `json:"organization"`
}
type Presets struct {
	Owners  []OwnerPreset `json:"owners"`
	Warning string        `json:"warning,omitempty"`
}

// Presets reads the account belonging to the configured GitHub token. Org
// memberships are supplemented by repository owners: fine-grained tokens
// return an empty membership list even when they can access org repositories.
// No sources are saved until the person chooses one of these presets.
func (s *Service) Presets(ctx context.Context, o Owner) (Presets, error) {
	out := Presets{Owners: []OwnerPreset{}}
	token, err := s.token(ctx, o)
	if err != nil {
		return out, err
	}
	var account struct {
		Login string `json:"login"`
	}
	if err := s.get(ctx, token, "user", &account); err != nil {
		return out, err
	}
	login, err := presetLogin(account.Login)
	if err != nil {
		return out, err
	}
	out.Owners = append(out.Owners, OwnerPreset{Login: login})
	organizations := map[string]bool{}
	add := func(raw string) error {
		name, err := presetLogin(raw)
		if err != nil {
			return err
		}
		if name != login {
			organizations[name] = true
		}
		return nil
	}
	membershipsErr := s.presetMemberships(ctx, token, add)
	repositoriesErr := s.presetRepositoryOwners(ctx, token, add)
	if membershipsErr != nil || repositoriesErr != nil {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		out.Warning = "Some organizations could not be listed with this GitHub connection. You can enter another owner manually."
	}
	names := make([]string, 0, len(organizations))
	for name := range organizations {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		out.Owners = append(out.Owners, OwnerPreset{Login: name, Organization: true})
	}
	return out, nil
}

func presetLogin(login string) (string, error) {
	if !ownerRE.MatchString(login) {
		return "", errors.New("GitHub returned an invalid account name")
	}
	return strings.ToLower(login), nil
}
func (s *Service) presetMemberships(ctx context.Context, token string, add func(string) error) error {
	for page := 1; page <= 100; page++ {
		var rows []struct {
			Login string `json:"login"`
		}
		if err := s.get(ctx, token, fmt.Sprintf("user/orgs?per_page=100&page=%d", page), &rows); err != nil {
			return err
		}
		for _, row := range rows {
			if err := add(row.Login); err != nil {
				return err
			}
		}
		if len(rows) < 100 {
			return nil
		}
	}
	return errors.New("too many organization memberships to list")
}
func (s *Service) presetRepositoryOwners(ctx context.Context, token string, add func(string) error) error {
	for page := 1; page <= 100; page++ {
		var rows []struct {
			Owner struct {
				Login string `json:"login"`
				Type  string `json:"type"`
			} `json:"owner"`
		}
		if err := s.get(ctx, token, fmt.Sprintf("user/repos?affiliation=owner,collaborator,organization_member&visibility=all&per_page=100&sort=full_name&direction=asc&page=%d", page), &rows); err != nil {
			return err
		}
		for _, row := range rows {
			if row.Owner.Type == "Organization" {
				if err := add(row.Owner.Login); err != nil {
					return err
				}
			}
		}
		if len(rows) < 100 {
			return nil
		}
	}
	return errors.New("too many repositories to list their organizations")
}
