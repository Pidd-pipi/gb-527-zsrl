export type ConflictType = 'station_capacity' | 'satellite_overlap' | 'band_mismatch' | 'duration_shortfall' | 'slew_buffer';
export type ResolutionStatus = 'detected' | 'proposed' | 'pending_review' | 'accepted' | 'rejected';
export type BackfillOutcome = 'success' | 'partial' | 'failed';
export type BackfillReviewStatus = 'archived' | 'needs_review';

export const CONFLICT_TYPES: ConflictType[] = ['station_capacity', 'satellite_overlap', 'band_mismatch', 'duration_shortfall', 'slew_buffer'];
export const BACKFILL_OUTCOMES: BackfillOutcome[] = ['success', 'partial', 'failed'];

export interface ScoreBreakdown {
  priority_loss: number;
  movement_distance_km: number;
  contact_duration_sec: number;
  resource_margin: number;
  total_score: number;
}

export interface ResolutionSuggestion {
  action_key: string;
  action_type: string;
  title: string;
  rationale: string;
  keep_window_ids: number[];
  move_window_ids: number[];
  target_station_id?: number;
  alternate_window_id?: number;
  requires_manual: boolean;
  score: ScoreBreakdown;
}

export interface ConflictEvidence {
  summary: string;
  window_facts: Array<Record<string, unknown>>;
  capacity: number;
  peak_concurrency: number;
  buffer_seconds: number;
  metadata: Record<string, unknown>;
}

export interface ContactBackfill {
  id: number;
  resolution_id: number;
  window_id: number;
  planned_start_at: string;
  planned_end_at: string;
  actual_start_at: string;
  actual_end_at: string;
  elevation_peak_deg: number;
  outcome: BackfillOutcome;
  result_note: string;
  start_deviation_sec: number;
  duration_deviation_sec: number;
  review_status: BackfillReviewStatus;
  recorded_by: string;
  created_at: string;
}

export interface BackfillSummary {
  count: number;
  latest_review_status?: BackfillReviewStatus;
  latest_outcome?: BackfillOutcome;
  flagged_window_ids: number[];
  latest_at?: string;
}

export interface ConflictResolution {
  id: number;
  conflict_key: string;
  window_ids: number[];
  conflict_type: ConflictType;
  evidence: ConflictEvidence;
  suggestions: ResolutionSuggestion[];
  selected_action?: ResolutionSuggestion;
  resolution_status: ResolutionStatus;
  resolved_by: string;
  review_note: string;
  version: number;
  backfill: BackfillSummary;
  resolved_at?: string;
  created_at: string;
  updated_at: string;
}

export interface DetectionResult {
  range_from: string;
  range_to: string;
  window_count: number;
  conflict_count: number;
  resolutions: ConflictResolution[];
}
