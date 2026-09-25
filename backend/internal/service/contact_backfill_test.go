package service

import (
	"errors"
	"fmt"
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

type backfillFixture struct {
	service   *ContactBackfillService
	conflicts *ConflictResolutionService
	accepted  dto.ConflictResolutionResponse
	proposed  dto.ConflictResolutionResponse
	windowIDs []uint
	base      time.Time
}

func newBackfillFixture(t *testing.T) backfillFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:backfill-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
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
		{StationID: station.ID, SatelliteID: assets[0].ID, StartAt: base, EndAt: base.Add(10 * time.Minute), Band: "S", WindowStatus: constants.WindowStatusSubmitted, Priority: 8, SourceVersion: "bf-source", Version: 1},
		{StationID: station.ID, SatelliteID: assets[1].ID, StartAt: base.Add(time.Minute), EndAt: base.Add(9 * time.Minute), Band: "S", WindowStatus: constants.WindowStatusSubmitted, Priority: 5, SourceVersion: "bf-source", Version: 1},
		{StationID: station.ID, SatelliteID: assets[0].ID, StartAt: base.Add(2 * time.Hour), EndAt: base.Add(2*time.Hour + 10*time.Minute), Band: "S", WindowStatus: constants.WindowStatusSubmitted, Priority: 7, SourceVersion: "bf-source", Version: 1},
		{StationID: station.ID, SatelliteID: assets[1].ID, StartAt: base.Add(2*time.Hour + time.Minute), EndAt: base.Add(2*time.Hour + 9*time.Minute), Band: "S", WindowStatus: constants.WindowStatusSubmitted, Priority: 4, SourceVersion: "bf-source", Version: 1},
	}
	if err := db.Create(&windows).Error; err != nil {
		t.Fatal(err)
	}
	conflictRepository := repository.NewConflictResolutionRepository(db)
	windowRepository := repository.NewContactWindowRepository(db)
	audit := NewAuditService(repository.NewSystemRepository(db))
	backfills := NewContactBackfillService(repository.NewContactBackfillRepository(db), conflictRepository, windowRepository, audit)
	conflicts := NewConflictResolutionService(conflictRepository, windowRepository, repository.NewGroundStationRepository(db), repository.NewSatelliteAssetRepository(db), audit, backfills, config.Weights{PriorityLoss: 4, MovementDistance: .02, ContactDuration: .003, ResourceMargin: 2})
	scheduler := dto.Actor{ID: 1, Username: "scheduler", Role: constants.RoleScheduler}
	reviewer := dto.Actor{ID: 2, Username: "reviewer", Role: constants.RoleReviewer}
	detected, err := conflicts.Detect(dto.DetectConflictsRequest{From: base.Add(-time.Minute).Format(time.RFC3339), To: base.Add(3 * time.Hour).Format(time.RFC3339)}, scheduler, "bf-detect")
	if err != nil {
		t.Fatal(err)
	}
	var target, proposed dto.ConflictResolutionResponse
	for _, resolution := range detected.Resolutions {
		if resolution.ConflictType != constants.ConflictTypeStationCapacity {
			continue
		}
		for _, windowID := range resolution.WindowIDs {
			if windowID == windows[0].ID {
				target = resolution
			}
			if windowID == windows[2].ID {
				proposed = resolution
			}
		}
	}
	if target.ID == 0 || proposed.ID == 0 {
		t.Fatalf("expected two station capacity conflicts, got %+v", detected.Resolutions)
	}
	fixture := backfillFixture{service: backfills, conflicts: conflicts, proposed: proposed, windowIDs: []uint{windows[0].ID, windows[1].ID}, base: base}
	submitted, err := conflicts.Submit(target.ID, dto.ConflictActionRequest{ExpectedVersion: target.Version}, scheduler, "bf-submit")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := conflicts.Review(target.ID, dto.ConflictActionRequest{ExpectedVersion: submitted.Version, Decision: constants.ResolutionStatusAccepted, ActionKey: target.Suggestions[0].ActionKey}, reviewer, "bf-accept")
	if err != nil {
		t.Fatal(err)
	}
	fixture.accepted = accepted
	return fixture
}

func backfillRequest(windowID uint, start, end time.Time) dto.CreateBackfillRequest {
	return dto.CreateBackfillRequest{
		WindowID: windowID, ActualStartAt: start.Format(time.RFC3339), ActualEndAt: end.Format(time.RFC3339),
		ActualElevationDeg: 55.5, Outcome: constants.BackfillOutcomeSuccess, ResultNote: "contact completed",
	}
}

func TestBackfillRequiresAcceptedResolution(t *testing.T) {
	fixture := newBackfillFixture(t)
	actor := dto.Actor{ID: 1, Username: "scheduler", Role: constants.RoleScheduler}
	_, err := fixture.service.Create(fixture.proposed.ID, backfillRequest(fixture.windowIDs[0], fixture.base, fixture.base.Add(10*time.Minute)), actor, "bf-early")
	var appError *AppError
	if !errors.As(err, &appError) || appError.Code != "invalid_state" {
		t.Fatalf("expected invalid_state for non-accepted resolution, got %v", err)
	}
	_, err = fixture.service.Create(fixture.accepted.ID, backfillRequest(9999, fixture.base, fixture.base.Add(10*time.Minute)), actor, "bf-foreign-window")
	if !errors.As(err, &appError) || appError.Code != "window_not_in_conflict" {
		t.Fatalf("expected window_not_in_conflict, got %v", err)
	}
}

func TestBackfillClassificationAndAppendOnlySummary(t *testing.T) {
	fixture := newBackfillFixture(t)
	actor := dto.Actor{ID: 1, Username: "scheduler", Role: constants.RoleScheduler}
	archived, err := fixture.service.Create(fixture.accepted.ID, backfillRequest(fixture.windowIDs[0], fixture.base.Add(4*time.Minute), fixture.base.Add(14*time.Minute)), actor, "bf-normal")
	if err != nil {
		t.Fatal(err)
	}
	if archived.ReviewStatus != constants.BackfillStatusArchived || archived.StartDeviationSec != 240 || archived.DurationDeviationSec != 0 {
		t.Fatalf("expected archived backfill with 240s start deviation, got %+v", archived)
	}
	flagged, err := fixture.service.Create(fixture.accepted.ID, backfillRequest(fixture.windowIDs[1], fixture.base.Add(12*time.Minute), fixture.base.Add(20*time.Minute)), actor, "bf-late")
	if err != nil {
		t.Fatal(err)
	}
	if flagged.ReviewStatus != constants.BackfillStatusNeedsReview || flagged.StartDeviationSec != 660 {
		t.Fatalf("expected needs_review for 11min start deviation, got %+v", flagged)
	}
	short, err := fixture.service.Create(fixture.accepted.ID, backfillRequest(fixture.windowIDs[1], fixture.base.Add(2*time.Minute), fixture.base.Add(16*time.Minute)), actor, "bf-short")
	if err != nil {
		t.Fatal(err)
	}
	if short.ReviewStatus != constants.BackfillStatusNeedsReview || short.DurationDeviationSec != 360 {
		t.Fatalf("expected needs_review for 6min duration gap, got %+v", short)
	}
	records, err := fixture.service.List(fixture.accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0].ReviewStatus != constants.BackfillStatusArchived {
		t.Fatalf("expected three append-only records with the first archived, got %+v", records)
	}
	loaded, err := fixture.conflicts.Get(fixture.accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	summary := loaded.Backfill
	if summary.Count != 3 || summary.LatestStatus != constants.BackfillStatusNeedsReview {
		t.Fatalf("expected count 3 and latest needs_review, got %+v", summary)
	}
	if len(summary.AbnormalWindowIDs) != 1 || summary.AbnormalWindowIDs[0] != fixture.windowIDs[1] {
		t.Fatalf("expected abnormal window %d, got %+v", fixture.windowIDs[1], summary.AbnormalWindowIDs)
	}
}

func TestBackfillBoundaryDeviationsArchive(t *testing.T) {
	fixture := newBackfillFixture(t)
	actor := dto.Actor{ID: 1, Username: "scheduler", Role: constants.RoleScheduler}
	exact, err := fixture.service.Create(fixture.accepted.ID, backfillRequest(fixture.windowIDs[0], fixture.base.Add(10*time.Minute), fixture.base.Add(25*time.Minute)), actor, "bf-boundary")
	if err != nil {
		t.Fatal(err)
	}
	if exact.StartDeviationSec != 600 || exact.DurationDeviationSec != 300 || exact.ReviewStatus != constants.BackfillStatusArchived {
		t.Fatalf("expected exact-limit deviations to archive, got %+v", exact)
	}
}
