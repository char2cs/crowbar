package usecases

type HealthStatus struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type HealthUsecase struct{}

func NewHealth() *HealthUsecase { return &HealthUsecase{} }

func (h *HealthUsecase) Check() HealthStatus {
	return HealthStatus{Status: "ok", Version: "dev"}
}
