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
	BackfillOutcomePartial = "partial"
	BackfillOutcomeFailed  = "failed"
)

var BackfillOutcomes = []string{
	BackfillOutcomeSuccess,
	BackfillOutcomePartial,
	BackfillOutcomeFailed,
}

const (
	BackfillReviewArchived    = "archived"
	BackfillReviewNeedsReview = "needs_review"
)

const (
	BackfillStartDeviationLimitSec    = 600
	BackfillDurationDeviationLimitSec = 300
)

func BackfillReviewStatus(startDeviationSec, durationDeviationSec int) string {
	if startDeviationSec > BackfillStartDeviationLimitSec || durationDeviationSec > BackfillDurationDeviationLimitSec {
		return BackfillReviewNeedsReview
	}
	return BackfillReviewArchived
}
