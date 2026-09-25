package dto

import "time"

type CreateBackfillRequest struct {
	WindowID           uint    `json:"window_id" validate:"required,gte=1"`
	ActualStartAt      string  `json:"actual_start_at" validate:"required"`
	ActualEndAt        string  `json:"actual_end_at" validate:"required"`
	ActualElevationDeg float64 `json:"actual_elevation_peak_deg" validate:"gte=0,lte=90"`
	Outcome            string  `json:"outcome" validate:"required,oneof=success failed"`
	ResultNote         string  `json:"result_note" validate:"omitempty,max=500"`
}

type BackfillResponse struct {
	ID                   uint      `json:"id"`
	ResolutionID         uint      `json:"resolution_id"`
	WindowID             uint      `json:"window_id"`
	ActualStartAt        time.Time `json:"actual_start_at"`
	ActualEndAt          time.Time `json:"actual_end_at"`
	ActualElevationDeg   float64   `json:"actual_elevation_peak_deg"`
	Outcome              string    `json:"outcome"`
	ResultNote           string    `json:"result_note"`
	StartDeviationSec    int       `json:"start_deviation_sec"`
	DurationDeviationSec int       `json:"duration_deviation_sec"`
	ReviewStatus         string    `json:"review_status"`
	RecordedBy           string    `json:"recorded_by"`
	CreatedAt            time.Time `json:"created_at"`
}

type BackfillSummary struct {
	Count             int    `json:"count"`
	LatestStatus      string `json:"latest_status,omitempty"`
	AbnormalWindowIDs []uint `json:"abnormal_window_ids"`
}
