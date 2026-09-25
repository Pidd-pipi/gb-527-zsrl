package service

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"satellite-contact-window-deconfliction/backend/internal/constants"
	"satellite-contact-window-deconfliction/backend/internal/dto"
	"satellite-contact-window-deconfliction/backend/internal/model"
	"satellite-contact-window-deconfliction/backend/internal/repository"
)

type ContactBackfillService struct {
	backfills   *repository.ContactBackfillRepository
	resolutions *repository.ConflictResolutionRepository
	windows     *repository.ContactWindowRepository
	audit       *AuditService
}

func NewContactBackfillService(backfills *repository.ContactBackfillRepository, resolutions *repository.ConflictResolutionRepository, windows *repository.ContactWindowRepository, audit *AuditService) *ContactBackfillService {
	return &ContactBackfillService{backfills: backfills, resolutions: resolutions, windows: windows, audit: audit}
}

func (service *ContactBackfillService) List(resolutionID uint) ([]dto.ContactBackfillResponse, error) {
	if _, err := service.resolutions.Get(resolutionID); err != nil {
		return nil, MapRepositoryError("conflict resolution", err)
	}
	records, err := service.backfills.ListByResolution(resolutionID)
	if err != nil {
		return nil, Internal("could not list contact backfills", err)
	}
	responses := make([]dto.ContactBackfillResponse, 0, len(records))
	for _, record := range records {
		responses = append(responses, backfillResponse(record))
	}
	return responses, nil
}

func (service *ContactBackfillService) Create(resolutionID uint, request dto.CreateContactBackfillRequest, actor dto.Actor, requestID string) (dto.ContactBackfillResponse, error) {
	actualStart, actualEnd, err := parseRange(request.ActualStartAt, request.ActualEndAt)
	if err != nil {
		return dto.ContactBackfillResponse{}, err
	}
	actualStart, actualEnd = actualStart.UTC(), actualEnd.UTC()
	var created model.ContactBackfill
	err = service.resolutions.DB().Transaction(func(tx *gorm.DB) error {
		resolution, err := service.resolutions.GetForUpdate(tx, resolutionID)
		if err != nil {
			return MapRepositoryError("conflict resolution", err)
		}
		if resolution.ResolutionStatus != constants.ResolutionStatusAccepted {
			return Conflict("invalid_state", "only accepted resolutions can receive execution backfill", nil)
		}
		windowIDs := []uint{}
		if err := json.Unmarshal([]byte(resolution.WindowIDsJSON), &windowIDs); err != nil {
			return Internal("stored window IDs are invalid", err)
		}
		if !containsWindow(windowIDs, request.WindowID) {
			return BadRequest("unknown_window", "window is not part of this conflict resolution")
		}
		window, err := service.windows.FindForUpdate(tx, request.WindowID)
		if err != nil {
			return MapRepositoryError("contact window", err)
		}
		startDeviation := absSeconds(actualStart.Sub(window.StartAt))
		durationDeviation := absSeconds(actualEnd.Sub(actualStart) - window.EndAt.Sub(window.StartAt))
		created = model.ContactBackfill{
			ResolutionID: resolution.ID, WindowID: window.ID,
			PlannedStartAt: window.StartAt, PlannedEndAt: window.EndAt,
			ActualStartAt: actualStart, ActualEndAt: actualEnd,
			ElevationPeakDeg: request.ElevationPeakDeg, Outcome: request.Outcome, ResultNote: request.ResultNote,
			StartDeviationSec: startDeviation, DurationDeviationSec: durationDeviation,
			ReviewStatus: constants.BackfillReviewStatus(startDeviation, durationDeviation),
			RecordedBy:   actor.Username,
		}
		if err := service.backfills.WithDB(tx).Create(&created); err != nil {
			return err
		}
		parameters := map[string]any{
			"window_id": window.ID, "outcome": request.Outcome, "review_status": created.ReviewStatus,
			"start_deviation_sec": startDeviation, "duration_deviation_sec": durationDeviation,
			"result_note_length": len(request.ResultNote),
		}
		after := map[string]any{"backfill_id": created.ID, "resolution_status": resolution.ResolutionStatus, "review_status": created.ReviewStatus}
		return service.audit.RecordTx(tx, actor, requestID, "conflict.backfilled", "conflict_resolution", auditID(resolution.ID), parameters, resolutionSummary(resolution), after)
	})
	if err != nil {
		return dto.ContactBackfillResponse{}, err
	}
	return backfillResponse(created), nil
}

func backfillResponse(record model.ContactBackfill) dto.ContactBackfillResponse {
	return dto.ContactBackfillResponse{
		ID: record.ID, ResolutionID: record.ResolutionID, WindowID: record.WindowID,
		PlannedStartAt: record.PlannedStartAt, PlannedEndAt: record.PlannedEndAt,
		ActualStartAt: record.ActualStartAt, ActualEndAt: record.ActualEndAt,
		ElevationPeakDeg: record.ElevationPeakDeg, Outcome: record.Outcome, ResultNote: record.ResultNote,
		StartDeviationSec: record.StartDeviationSec, DurationDeviationSec: record.DurationDeviationSec,
		ReviewStatus: record.ReviewStatus, RecordedBy: record.RecordedBy, CreatedAt: record.CreatedAt,
	}
}

func summarizeBackfills(records []model.ContactBackfill) dto.BackfillSummary {
	summary := dto.BackfillSummary{Count: len(records), FlaggedWindowIDs: []uint{}}
	if len(records) == 0 {
		return summary
	}
	latest := records[len(records)-1]
	summary.LatestReviewStatus = latest.ReviewStatus
	summary.LatestOutcome = latest.Outcome
	latestAt := latest.CreatedAt
	summary.LatestAt = &latestAt
	flagged := map[uint]bool{}
	for _, record := range records {
		if record.ReviewStatus == constants.BackfillReviewNeedsReview && !flagged[record.WindowID] {
			flagged[record.WindowID] = true
			summary.FlaggedWindowIDs = append(summary.FlaggedWindowIDs, record.WindowID)
		}
	}
	return summary
}

func containsWindow(windowIDs []uint, id uint) bool {
	for _, windowID := range windowIDs {
		if windowID == id {
			return true
		}
	}
	return false
}

func absSeconds(value time.Duration) int {
	if value < 0 {
		return int(-value / time.Second)
	}
	return int(value / time.Second)
}
