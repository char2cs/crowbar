package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Repository is the repository interface for the Repository aggregate.
type Repository interface {
	Create(ctx context.Context, projectID string, name string, path string) (domain.Repository, error)
	List(ctx context.Context, projectID string) ([]domain.Repository, error)
	Get(ctx context.Context, id string) (domain.Repository, error)
	Delete(ctx context.Context, id string) error
}

type repo struct {
	db *gormdb.DB
}

// New constructs a GORM-backed Repository repository and runs AutoMigrate.
func New(
	db *gormdb.DB,
) (Repository, error) {
	if err := db.AutoMigrate(&domain.Repository{}); err != nil {
		return nil, fmt.Errorf("repository repo: migrate: %w", err)
	}
	return &repo{db: db}, nil
}

func (r *repo) Create(ctx context.Context, projectID string, name string, path string) (domain.Repository, error) {
	rec := domain.Repository{
		ID:        uuid.New().String(),
		ProjectID: projectID,
		Name:      name,
		Path:      path,
		CreatedAt: time.Now().UTC(),
	}
	if err := r.db.WithContext(ctx).Create(&rec).Error; err != nil {
		return domain.Repository{}, fmt.Errorf("repository repo: create: %w", err)
	}
	return rec, nil
}

func (r *repo) List(ctx context.Context, projectID string) ([]domain.Repository, error) {
	var recs []domain.Repository
	if err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at ASC").
		Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("repository repo: list: %w", err)
	}
	return recs, nil
}

func (r *repo) Get(ctx context.Context, id string) (domain.Repository, error) {
	var rec domain.Repository
	err := r.db.WithContext(ctx).First(&rec, "id = ?", id).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return domain.Repository{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Repository{}, fmt.Errorf("repository repo: get: %w", err)
	}
	return rec, nil
}

func (r *repo) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&domain.Repository{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("repository repo: delete: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
