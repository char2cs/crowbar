package app

import (
	"github.com/rabbytesoftware/crowbar/api/internal/app/hub"
	"github.com/rabbytesoftware/crowbar/api/internal/app/usecases"
)

type Container struct {
	Hub    *hub.Hub
	Health *usecases.HealthUsecase
}

func New() (*Container, error) {
	return &Container{
		Hub:    hub.New(),
		Health: usecases.NewHealth(),
	}, nil
}
