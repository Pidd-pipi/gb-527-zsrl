package service

import (
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"satellite-contact-window-deconfliction/backend/internal/config"
	"satellite-contact-window-deconfliction/backend/internal/constants"
	"satellite-contact-window-deconfliction/backend/internal/dto"
	"satellite-contact-window-deconfliction/backend/internal/model"
	"satellite-contact-window-deconfliction/backend/internal/repository"
)

func TestBackfillRecordsDeviationAndKeepsHistory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:contact-backfill?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.GroundStation{}, &model.SatelliteAsset{}, &model.ContactWindow{}, &model.ConflictResolution{}, &model.ContactBackfill{}, &model.AuditEvent{}); err != nil {
		t.Fatal(err)
	}
	station := model.GroundStation{StationCode: "BF-GS", Name: "Backfill", AntennaCount: 1, SupportedBandsJSON: `["S"]`, StationStatus: "active", Version: 1}
	assets := []model.SatelliteAsset{
		{SatelliteCode: "BF-A", Name: "A", SupportedBandsJSON: `["S"]`, MinimumContactSec: 60, AssetStatus: "active", PriorityWeight: 2, Version: 1},
		{SatelliteCode: "BF-B", Name: "B", SupportedBandsJSON: `["S"]`, MinimumContactSec: 60, AssetStatus: "active", PriorityWeight: 1, Version: 1},
	}
	if err := db.Create(&station).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&assets).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Second)
	windows := []model.ContactWindow{
		{StationID: station.ID, SatelliteID: assets[0].ID, StartAt: base, EndAt: base.Add(10 * time.Minute), Band: "S", WindowStatus: constants.WindowStatusSubmitted, Priority: 8, SourceVersion: "test-source", Version: 1},
		{StationID: station.ID, SatelliteID: assets[1].ID, StartAt: base.Add(time.Minute), EndAt: base.Add(9 * time.Minute), Band: "S", WindowStatus: constants.WindowStatusSubmitted, Priority: 5, SourceVersion: "test-source", Version: 1},
	}
	if err := db.Create(&windows).Error; err != nil {
		t.Fatal(err)
	}
	conflictRepository := repository.NewConflictResolutionRepository(db)
	windowRepository := repository.NewContactWindowRepository(db)
	backfillRepository := repository.NewContactBackfillRepository(db)
	audit := NewAuditService(repository.NewSystemRepository(db))
	conflicts := NewConflictResolutionService(conflictRepository, windowRepository, repository.NewGroundStationRepository(db), repository.NewSatelliteAssetRepository(db), backfillRepository, audit, config.Weights{PriorityLoss: 4, MovementDistance: .02, ContactDuration: .003, ResourceMargin: 2})
	backfills := NewContactBackfillService(backfillRepository, conflictRepository, windowRepository, audit)
	scheduler := dto.Actor{ID: 1, Username: "scheduler", Role: constants.RoleScheduler}
	detected, err := conflicts.Detect(dto.DetectConflictsRequest{From: base.Add(-time.Minute).Format(time.RFC3339), To: base.Add(time.Hour).Format(time.RFC3339)}, scheduler, "bf-detect")
	if err != nil {
		t.Fatal(err)
	}
	var target dto.ConflictResolutionResponse
	for _, resolution := range detected.Resolutions {
		if resolution.ConflictType == constants.ConflictTypeStationCapacity {
			target = resolution
			break
		}
	}
	if target.ID == 0 {
		t.Fatal("expected a station capacity conflict")
	}
	request := dto.CreateContactBackfillRequest{WindowID: windows[0].ID, ActualStartAt: base.Format(time.RFC3339), ActualEndAt: base.Add(10 * time.Minute).Format(time.RFC3339), ElevationPeakDeg: 55, Outcome: constants.BackfillOutcomeSuccess}
	if _, err := backfills.Create(target.ID, request, scheduler, "bf-early"); err == nil {
		t.Fatal("expected backfill on proposed resolution to fail")
	} else {
		var appError *AppError
		if !errors.As(err, &appError) || appError.Code != "invalid_state" {
			t.Fatalf("expected invalid_state, got %v", err)
		}
	}
	target, err = conflicts.Submit(target.ID, dto.ConflictActionRequest{ExpectedVersion: target.Version}, scheduler, "bf-submit")
	if err != nil {
		t.Fatal(err)
	}
	reviewer := dto.Actor{ID: 2, Username: "reviewer", Role: constants.RoleReviewer}
	target, err = conflicts.Review(target.ID, dto.ConflictActionRequest{ExpectedVersion: target.Version, Decision: constants.ResolutionStatusAccepted, ActionKey: target.Suggestions[0].ActionKey}, reviewer, "bf-accept")
	if err != nil {
		t.Fatal(err)
	}
	late := dto.CreateContactBackfillRequest{WindowID: windows[0].ID, ActualStartAt: base.Add(12 * time.Minute).Format(time.RFC3339), ActualEndAt: base.Add(22 * time.Minute).Format(time.RFC3339), ElevationPeakDeg: 58.2, Outcome: constants.BackfillOutcomePartial, ResultNote: "acquired 12 minutes late"}
	first, err := backfills.Create(target.ID, late, scheduler, "bf-late")
	if err != nil {
		t.Fatal(err)
	}
	if first.ReviewStatus != constants.BackfillReviewNeedsReview || first.StartDeviationSec != 720 || first.DurationDeviationSec != 0 {
		t.Fatalf("expected needs_review with 720s start deviation, got %+v", first)
	}
	onTime := dto.CreateContactBackfillRequest{WindowID: windows[0].ID, ActualStartAt: base.Add(2 * time.Minute).Format(time.RFC3339), ActualEndAt: base.Add(12 * time.Minute).Format(time.RFC3339), ElevationPeakDeg: 61, Outcome: constants.BackfillOutcomeSuccess}
	second, err := backfills.Create(target.ID, onTime, scheduler, "bf-ontime")
	if err != nil {
		t.Fatal(err)
	}
	if second.ReviewStatus != constants.BackfillReviewArchived {
		t.Fatalf("expected archived, got %+v", second)
	}
	stretched := dto.CreateContactBackfillRequest{WindowID: windows[1].ID, ActualStartAt: base.Add(time.Minute).Format(time.RFC3339), ActualEndAt: base.Add(15 * time.Minute).Format(time.RFC3339), ElevationPeakDeg: 40, Outcome: constants.BackfillOutcomeSuccess}
	third, err := backfills.Create(target.ID, stretched, scheduler, "bf-stretched")
	if err != nil {
		t.Fatal(err)
	}
	if third.ReviewStatus != constants.BackfillReviewNeedsReview || third.DurationDeviationSec != 360 {
		t.Fatalf("expected needs_review with 360s duration deviation, got %+v", third)
	}
	records, err := backfills.List(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 backfill records, got %d", len(records))
	}
	if records[0].ID != first.ID || records[0].ReviewStatus != constants.BackfillReviewNeedsReview || records[0].StartDeviationSec != 720 {
		t.Fatalf("later entries must not rewrite the first record, got %+v", records[0])
	}
	resolution, err := conflicts.Get(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Backfill.Count != 3 || resolution.Backfill.LatestReviewStatus != constants.BackfillReviewNeedsReview || resolution.Backfill.LatestOutcome != constants.BackfillOutcomeSuccess {
		t.Fatalf("unexpected backfill summary %+v", resolution.Backfill)
	}
	if len(resolution.Backfill.FlaggedWindowIDs) != 2 || resolution.Backfill.FlaggedWindowIDs[0] != windows[0].ID || resolution.Backfill.FlaggedWindowIDs[1] != windows[1].ID {
		t.Fatalf("expected flagged windows [%d %d], got %v", windows[0].ID, windows[1].ID, resolution.Backfill.FlaggedWindowIDs)
	}
	stranger := dto.CreateContactBackfillRequest{WindowID: 99999, ActualStartAt: base.Format(time.RFC3339), ActualEndAt: base.Add(10 * time.Minute).Format(time.RFC3339), Outcome: constants.BackfillOutcomeSuccess}
	if _, err := backfills.Create(target.ID, stranger, scheduler, "bf-stranger"); err == nil {
		t.Fatal("expected backfill for foreign window to fail")
	} else {
		var appError *AppError
		if !errors.As(err, &appError) || appError.Code != "unknown_window" {
			t.Fatalf("expected unknown_window, got %v", err)
		}
	}
	records, err = backfills.List(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("failed validation must not append records, got %d", len(records))
	}
}
