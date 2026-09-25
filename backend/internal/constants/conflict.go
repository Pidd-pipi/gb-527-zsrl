package constants

const (
	ConflictTypeStationCapacity   = "station_capacity"
	ConflictTypeSatelliteOverlap  = "satellite_overlap"
	ConflictTypeBandMismatch      = "band_mismatch"
	ConflictTypeDurationShortfall = "duration_shortfall"
	ConflictTypeSlewBuffer        = "slew_buffer"
)

var ConflictTypes = []string{
	ConflictTypeStationCapacity,
	ConflictTypeSatelliteOverlap,
	ConflictTypeBandMismatch,
	ConflictTypeDurationShortfall,
	ConflictTypeSlewBuffer,
}

const (
	ResolutionStatusDetected      = "detected"
	ResolutionStatusProposed      = "proposed"
	ResolutionStatusPendingReview = "pending_review"
	ResolutionStatusAccepted      = "accepted"
	ResolutionStatusRejected      = "rejected"
)

func CanTransitionResolution(from, to string) bool {
	switch from {
	case ResolutionStatusDetected:
		return to == ResolutionStatusProposed
	case ResolutionStatusProposed:
		return to == ResolutionStatusPendingReview
	case ResolutionStatusPendingReview:
		return to == ResolutionStatusAccepted || to == ResolutionStatusRejected
	default:
		return false
	}
}

const (
	BackfillOutcomeSuccess = "success"
	BackfillOutcomeFailed  = "failed"
)

const (
	BackfillStatusArchived    = "archived"
	BackfillStatusNeedsReview = "needs_review"
)

const (
	BackfillStartDeviationLimitSec    = 600
	BackfillDurationDeviationLimitSec = 300
)

func ClassifyBackfill(startDeviationSec, durationDeviationSec int) string {
	if startDeviationSec > BackfillStartDeviationLimitSec || durationDeviationSec > BackfillDurationDeviationLimitSec {
		return BackfillStatusNeedsReview
	}
	return BackfillStatusArchived
}
