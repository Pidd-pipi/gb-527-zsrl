package repository

import (
	"fmt"

	"gorm.io/gorm"

	"satellite-contact-window-deconfliction/backend/internal/model"
)

type ContactBackfillRepository struct{ db *gorm.DB }

func NewContactBackfillRepository(db *gorm.DB) *ContactBackfillRepository {
	return &ContactBackfillRepository{db: db}
}

func (repository *ContactBackfillRepository) WithDB(db *gorm.DB) *ContactBackfillRepository {
	return &ContactBackfillRepository{db: db}
}

func (repository *ContactBackfillRepository) Create(backfill *model.ContactBackfill) error {
	if err := repository.db.Create(backfill).Error; err != nil {
		return fmt.Errorf("create contact backfill: %w", err)
	}
	return nil
}

func (repository *ContactBackfillRepository) ListByResolution(resolutionID uint) ([]model.ContactBackfill, error) {
	var backfills []model.ContactBackfill
	if err := repository.db.Where("resolution_id = ?", resolutionID).Order("id ASC").Find(&backfills).Error; err != nil {
		return nil, fmt.Errorf("list contact backfills for resolution %d: %w", resolutionID, err)
	}
	return backfills, nil
}

func (repository *ContactBackfillRepository) ListByResolutions(resolutionIDs []uint) ([]model.ContactBackfill, error) {
	if len(resolutionIDs) == 0 {
		return nil, nil
	}
	var backfills []model.ContactBackfill
	if err := repository.db.Where("resolution_id IN ?", resolutionIDs).Order("id ASC").Find(&backfills).Error; err != nil {
		return nil, fmt.Errorf("list contact backfills for resolutions: %w", err)
	}
	return backfills, nil
}
