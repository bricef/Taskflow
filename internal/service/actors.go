package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/repo"
)

func (s *Service) CreateActor(ctx context.Context, params model.CreateActorParams) (model.Actor, error) {
	if err := params.Validate(); err != nil {
		return model.Actor{}, err
	}

	var actor model.Actor
	err := s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		actor, err = s.store.ActorInsert(ctx, tx, model.Actor{
			Name:        params.Name,
			DisplayName: params.DisplayName,
			Type:        params.Type,
			Role:        params.Role,
			APIKeyHash:  params.APIKeyHash,
		})
		return err
	})
	return actor, err
}

func (s *Service) GetActor(ctx context.Context, name string) (model.Actor, error) {
	return s.store.ActorGet(ctx, name)
}

func (s *Service) GetActorByAPIKeyHash(ctx context.Context, hash string) (model.Actor, error) {
	return s.store.ActorGetByAPIKeyHash(ctx, hash)
}

// RotateActorKey generates a new API key for the actor, stores the hash,
// and returns the plaintext key (shown once).
func (s *Service) RotateActorKey(ctx context.Context, name string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating API key: %w", err)
	}
	apiKey := base64.RawURLEncoding.EncodeToString(b)
	hash := sha256Hex(apiKey)
	if err := s.store.ActorUpdateKeyHash(ctx, name, hash); err != nil {
		return "", err
	}
	return apiKey, nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func (s *Service) ListActors(ctx context.Context) ([]model.Actor, error) {
	return s.store.ActorList(ctx)
}

func (s *Service) UpdateActor(ctx context.Context, params model.UpdateActorParams) (model.Actor, error) {
	if params.Role.Set {
		if err := model.ValidateRole(params.Role.Value); err != nil {
			return model.Actor{}, err
		}
	}

	var actor model.Actor
	err := s.store.InTransaction(ctx, func(tx repo.Tx) error {
		var err error
		actor, err = s.store.ActorUpdate(ctx, tx, params)
		return err
	})
	return actor, err
}
