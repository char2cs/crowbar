package project

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type repo struct {
	db *gormdb.DB
}

// Project is the repository interface for the Project aggregate.
type Project interface {
	Create(ctx context.Context, name string) (domain.Project, error)
	List(ctx context.Context) ([]domain.Project, error)
	Get(ctx context.Context, id string) (domain.Project, error)
	Delete(ctx context.Context, id string) error
}

// New constructs a GORM-backed Project repository and runs AutoMigrate.
func New(
	db *gormdb.DB,
) (Project, error) {
	if err := db.AutoMigrate(&domain.Project{}); err != nil {
		return nil, fmt.Errorf("project repo: migrate: %w", err)
	}
	return &repo{db: db}, nil
}

func (r *repo) Create(ctx context.Context, name string) (domain.Project, error) {
	p := domain.Project{
		ID:        uuid.New().String(),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	if err := r.db.WithContext(ctx).Create(&p).Error; err != nil {
		return domain.Project{}, fmt.Errorf("project repo: create: %w", err)
	}
	return p, nil
}

func (r *repo) List(ctx context.Context) ([]domain.Project, error) {
	var projects []domain.Project
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("project repo: list: %w", err)
	}
	return projects, nil
}

func (r *repo) Get(ctx context.Context, id string) (domain.Project, error) {
	var p domain.Project
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return domain.Project{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("project repo: get: %w", err)
	}
	return p, nil
}

func (r *repo) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&domain.Project{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("project repo: delete: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
