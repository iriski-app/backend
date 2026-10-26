package service

import "github.com/iriski-app/backend/internal/storage/postgres"

type Service struct {
	Store *postgres.Store
}

func NewService(store *postgres.Store) *Service {
	return &Service{
		Store: store,
	}
}
