package app

import (
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
)

type Container struct {
	Hub      *hub.Hub
	Health   *usecases.HealthUsecase
	Fixtures *fixtures.Store
	WSHubs   *WSHubSet
}

type WSHubSet struct {
	Workspaces *wshub.Hub
	Git        *wshub.Hub
	Files      *wshub.Hub
	Chat       *wshub.Hub
	Terminal   *wshub.Hub
	Daemon     *wshub.Hub
}

func New() (*Container, error) {
	store, err := fixtures.Load()
	if err != nil {
		return nil, err
	}
	return &Container{
		Hub:      hub.New(),
		Health:   usecases.NewHealth(),
		Fixtures: store,
		WSHubs: &WSHubSet{
			Workspaces: wshub.New(),
			Git:        wshub.New(),
			Files:      wshub.New(),
			Chat:       wshub.New(),
			Terminal:   wshub.New(),
			Daemon:     wshub.New(),
		},
	}, nil
}
