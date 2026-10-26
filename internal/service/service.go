package service

import "github.com/iriski-app/backend/internal/storage/postgres"

type Service struct {
	store *postgres.Store
}

func NewService(store *postgres.Store) *Service {
	return &Service{
		store: store,
	}
}
