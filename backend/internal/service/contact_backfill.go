package service

import (
	"sort"
	"time"

	"gorm.io/gorm"

	"satellite-contact-window-deconfliction/backend/internal/constants"
	"satellite-contact-window-deconfliction/backend/internal/dto"
	"satellite-contact-window-deconfliction/backend/internal/model"
	"satellite-contact-window-deconfliction/backend/internal/repository"
)

type ContactBackfillService struct {
	repository  *repository.ContactBackfillRepository
	resolutions *repository.ConflictResolutionRepository
	windows     *repository.ContactWindowRepository
	audit       *AuditService
}

func NewContactBackfillService(backfills *repository.ContactBackfillRepository, resolutions *repository.ConflictResolutionRepository, windows *repository.ContactWindowRepository, audit *AuditService) *ContactBackfillService {
	return &ContactBackfillService{repository: backfills, resolutions: resolutions, windows: windows, audit: audit}
}

func (service *ContactBackfillService) Create(resolutionID uint, request dto.CreateBackfillRequest, actor dto.Actor, requestID string) (dto.BackfillResponse, error) {
	resolution, err := service.resolutions.Get(resolutionID)
	if err != nil {
		return dto.BackfillResponse{}, MapRepositoryError("conflict resolution", err)
	}
	if resolution.ResolutionStatus != constants.ResolutionStatusAccepted {
		return dto.BackfillResponse{}, Conflict("invalid_state", "only accepted resolutions can receive execution backfill", nil)
	}
	if request.Outcome != constants.BackfillOutcomeSuccess && request.Outcome != constants.BackfillOutcomeFailed {
		return dto.BackfillResponse{}, BadRequest("invalid_outcome", "outcome must be success or failed")
	}
	windowIDs, err := decodeWindowIDs(resolution.WindowIDsJSON)
	if err != nil {
		return dto.BackfillResponse{}, err
	}
	if !containsWindow(windowIDs, request.WindowID) {
		return dto.BackfillResponse{}, BadRequest("window_not_in_conflict", "backfill window must be one of the conflict windows")
	}
	actualStart, err := time.Parse(time.RFC3339, request.ActualStartAt)
	if err != nil {
		return dto.BackfillResponse{}, BadRequest("invalid_time_range", "actual_start_at must be RFC3339")
	}
	actualEnd, err := time.Parse(time.RFC3339, request.ActualEndAt)
	if err != nil {
		return dto.BackfillResponse{}, BadRequest("invalid_time_range", "actual_end_at must be RFC3339")
	}
	actualStart, actualEnd = actualStart.UTC(), actualEnd.UTC()
	if !actualEnd.After(actualStart) {
		return dto.BackfillResponse{}, BadRequest("invalid_time_range", "actual_end_at must be after actual_start_at")
	}
	window, err := service.windows.Get(request.WindowID)
	if err != nil {
		return dto.BackfillResponse{}, MapRepositoryError("contact window", err)
	}
	startDeviation := absSeconds(int(actualStart.Sub(window.StartAt).Seconds()))
	durationDeviation := absSeconds(int(actualEnd.Sub(actualStart).Seconds()) - window.DurationSec())
	backfill := model.ContactBackfill{
		ResolutionID: resolution.ID, WindowID: request.WindowID,
		ActualStartAt: actualStart, ActualEndAt: actualEnd, ActualElevationDeg: request.ActualElevationDeg,
		Outcome: request.Outcome, ResultNote: request.ResultNote,
		StartDeviationSec: startDeviation, DurationDeviationSec: durationDeviation,
		ReviewStatus: constants.ClassifyBackfill(startDeviation, durationDeviation),
		RecordedBy:   actor.Username,
	}
	err = service.repository.DB().Transaction(func(tx *gorm.DB) error {
		if err := service.repository.WithDB(tx).Create(&backfill); err != nil {
			return err
		}
		parameters := map[string]any{
			"resolution_id": resolution.ID, "window_id": request.WindowID, "outcome": request.Outcome,
			"start_deviation_sec": startDeviation, "duration_deviation_sec": durationDeviation,
			"review_status": backfill.ReviewStatus, "result_note_length": len(request.ResultNote),
		}
		return service.audit.RecordTx(tx, actor, requestID, "conflict.backfilled", "conflict_resolution", auditID(resolution.ID), parameters, nil, map[string]any{"backfill_id": backfill.ID, "review_status": backfill.ReviewStatus})
	})
	if err != nil {
		return dto.BackfillResponse{}, Internal("could not record execution backfill", err)
	}
	return backfillResponse(backfill), nil
}

func (service *ContactBackfillService) List(resolutionID uint) ([]dto.BackfillResponse, error) {
	if _, err := service.resolutions.Get(resolutionID); err != nil {
		return nil, MapRepositoryError("conflict resolution", err)
	}
	backfills, err := service.repository.ListByResolution(resolutionID)
	if err != nil {
		return nil, Internal("could not list execution backfills", err)
	}
	responses := make([]dto.BackfillResponse, 0, len(backfills))
	for _, backfill := range backfills {
		responses = append(responses, backfillResponse(backfill))
	}
	return responses, nil
}

func (service *ContactBackfillService) Summaries(resolutionIDs []uint) (map[uint]dto.BackfillSummary, error) {
	summaries := map[uint]dto.BackfillSummary{}
	backfills, err := service.repository.ListForResolutions(resolutionIDs)
	if err != nil {
		return nil, Internal("could not summarize execution backfills", err)
	}
	abnormal := map[uint]map[uint]bool{}
	for _, backfill := range backfills {
		summary := summaries[backfill.ResolutionID]
		summary.Count++
		summary.LatestStatus = backfill.ReviewStatus
		summaries[backfill.ResolutionID] = summary
		if backfill.ReviewStatus == constants.BackfillStatusNeedsReview {
			if abnormal[backfill.ResolutionID] == nil {
				abnormal[backfill.ResolutionID] = map[uint]bool{}
			}
			abnormal[backfill.ResolutionID][backfill.WindowID] = true
		}
	}
	for resolutionID, windows := range abnormal {
		summary := summaries[resolutionID]
		summary.AbnormalWindowIDs = make([]uint, 0, len(windows))
		for windowID := range windows {
			summary.AbnormalWindowIDs = append(summary.AbnormalWindowIDs, windowID)
		}
		sort.Slice(summary.AbnormalWindowIDs, func(i, j int) bool { return summary.AbnormalWindowIDs[i] < summary.AbnormalWindowIDs[j] })
		summaries[resolutionID] = summary
	}
	for resolutionID, summary := range summaries {
		if summary.AbnormalWindowIDs == nil {
			summary.AbnormalWindowIDs = []uint{}
			summaries[resolutionID] = summary
		}
	}
	return summaries, nil
}

func backfillResponse(backfill model.ContactBackfill) dto.BackfillResponse {
	return dto.BackfillResponse{
		ID: backfill.ID, ResolutionID: backfill.ResolutionID, WindowID: backfill.WindowID,
		ActualStartAt: backfill.ActualStartAt, ActualEndAt: backfill.ActualEndAt, ActualElevationDeg: backfill.ActualElevationDeg,
		Outcome: backfill.Outcome, ResultNote: backfill.ResultNote,
		StartDeviationSec: backfill.StartDeviationSec, DurationDeviationSec: backfill.DurationDeviationSec,
		ReviewStatus: backfill.ReviewStatus, RecordedBy: backfill.RecordedBy, CreatedAt: backfill.CreatedAt,
	}
}

func absSeconds(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func containsWindow(windowIDs []uint, id uint) bool {
	for _, candidate := range windowIDs {
		if candidate == id {
			return true
		}
	}
	return false
}
