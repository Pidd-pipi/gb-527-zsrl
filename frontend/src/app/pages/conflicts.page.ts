import { ChangeDetectionStrategy, Component, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import { finalize } from 'rxjs';
import { ApiService } from '../api/api.service';
import { ConflictResolution, ConflictType, CONFLICT_TYPES, ContactBackfill, BackfillOutcome } from '../types/conflict';
import { ResolutionComparePanelComponent } from '../components/common/resolution-compare-panel.component';
import { WindowStatusBadgeComponent } from '../components/common/window-status-badge.component';
import { ConflictDetectionHook, apiErrorMessage } from '../hooks/use-conflict-detection';
import { useAuth } from '../hooks/use-auth';
import { formatUtc, toLocalInput } from '../utils/date';

@Component({
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, MatButtonModule, MatFormFieldModule, MatInputModule, MatSelectModule, MatSnackBarModule, ResolutionComparePanelComponent, WindowStatusBadgeComponent],
  template: `
    <div class="page wide">
      <header class="page-head"><div><span class="eyebrow">Evidence and human decision</span><h1>Conflict resolution</h1><p>{{ resolutions().length }} recorded conflict groups</p></div></header>
      <div class="error-banner" *ngIf="error() || detection.error()"><span>{{ error() || detection.error() }}</span><button mat-button (click)="load()">Reload</button></div>
      <form class="detection-bar" [formGroup]="rangeForm" (ngSubmit)="detect()">
        <mat-form-field appearance="outline"><mat-label>Range start</mat-label><input matInput type="datetime-local" formControlName="from"></mat-form-field>
        <mat-form-field appearance="outline"><mat-label>Range end</mat-label><input matInput type="datetime-local" formControlName="to"></mat-form-field>
        <mat-form-field appearance="outline"><mat-label>Conflict type</mat-label><mat-select [value]="typeFilter()" (selectionChange)="filterType($event.value)"><mat-option value="">All types</mat-option><mat-option *ngFor="let type of types" [value]="type">{{ typeLabel(type) }}</mat-option></mat-select></mat-form-field>
        <button *ngIf="auth.canPlan()" mat-flat-button color="primary" type="submit" [disabled]="rangeForm.invalid || detection.running()"><span class="spinner" *ngIf="detection.running()"></span>{{ detection.running() ? 'Scanning' : 'Detect conflicts' }}</button>
      </form>
      <div class="detection-result" *ngIf="detection.result() as result"><strong>{{ result.conflict_count }} conflicts</strong><span>{{ result.window_count }} windows scanned</span><span>{{ format(result.range_from) }} – {{ format(result.range_to) }}</span></div>
      <div class="conflict-layout">
        <section class="conflict-list surface">
          <button *ngFor="let resolution of resolutions()" type="button" [class.active]="selected()?.id === resolution.id" (click)="select(resolution)">
            <span class="type-mark">{{ typeCode(resolution.conflict_type) }}</span><span><strong>{{ typeLabel(resolution.conflict_type) }}</strong><small>#{{ resolution.id }} · windows {{ resolution.window_ids.join(', ') }}</small><small class="backfill-meta" *ngIf="resolution.backfill.count > 0">{{ resolution.backfill.count }} backfill{{ resolution.backfill.count > 1 ? 's' : '' }} · {{ resolution.backfill.latest_status }}<em *ngIf="resolution.backfill.abnormal_window_ids?.length"> ⚠ windows {{ resolution.backfill.abnormal_window_ids.join(', ') }}</em></small></span><app-window-status-badge [status]="resolution.resolution_status" />
          </button><div class="empty" *ngIf="!resolutions().length">Run detection for the selected UTC range.</div>
        </section>
        <ng-container *ngIf="selected() as resolution; else noSelection">
          <section class="evidence surface">
            <div class="panel-head"><span class="eyebrow">Conflict evidence</span><h2>{{ typeLabel(resolution.conflict_type) }}</h2><app-window-status-badge [status]="resolution.resolution_status" /></div>
            <p class="summary">{{ resolution.evidence.summary }}</p>
            <dl><div><dt>Capacity</dt><dd>{{ resolution.evidence.capacity }}</dd></div><div><dt>Peak</dt><dd>{{ resolution.evidence.peak_concurrency }}</dd></div><div><dt>Buffer</dt><dd>{{ resolution.evidence.buffer_seconds }} sec</dd></div><div><dt>Version</dt><dd>v{{ resolution.version }}</dd></div></dl>
            <table><thead><tr><th>Window</th><th>Resource</th><th>Interval</th><th>Band</th><th>Lock</th></tr></thead><tbody><tr *ngFor="let fact of resolution.evidence.window_facts"><td class="code">#{{ fact['id'] }}</td><td>S{{ fact['station_id'] }} / A{{ fact['satellite_id'] }}</td><td>{{ formatFact(fact['start_at']) }}<br><span>{{ fact['duration_sec'] }} sec</span></td><td>{{ fact['band'] }}</td><td>{{ fact['locked'] ? 'locked' : 'open' }}</td></tr></tbody></table>
            <details><summary>Detection metadata</summary><pre>{{ resolution.evidence.metadata | json }}</pre></details>
          </section>
          <section class="decision">
            <div class="section-title"><h2>Ranked options</h2><span>Stable score order</span></div>
            <app-resolution-compare-panel [resolution]="resolution" [selectedKey]="selectedKey()" [readonly]="resolution.resolution_status === 'accepted' || resolution.resolution_status === 'rejected'" (selectedKeyChange)="selectedKey.set($event)" />
            <div class="workflow surface">
              <ng-container *ngIf="resolution.resolution_status === 'proposed' && auth.canPlan()"><p>Submit this evidence set and its ranked options for independent review.</p><button mat-flat-button color="primary" (click)="submitForReview(resolution)">Submit for review</button></ng-container>
              <form *ngIf="resolution.resolution_status === 'pending_review' && auth.canReview()" [formGroup]="reviewForm"><mat-form-field appearance="outline"><mat-label>Review note</mat-label><textarea matInput rows="3" formControlName="note"></textarea></mat-form-field><div class="action-row"><button mat-stroked-button color="warn" type="button" (click)="review(resolution, 'rejected')">Reject</button><button mat-flat-button color="primary" type="button" [disabled]="!selectedKey()" (click)="review(resolution, 'accepted')">Accept selected</button></div></form>
              <div *ngIf="resolution.resolution_status === 'accepted' || resolution.resolution_status === 'rejected'"><strong>Decision {{ resolution.resolution_status }}</strong><p>{{ resolution.review_note || 'No review note recorded.' }}</p><span class="muted">{{ resolution.resolved_by }} · {{ resolution.resolved_at ? format(resolution.resolved_at) : '' }}</span></div>
              <p *ngIf="resolution.resolution_status === 'pending_review' && !auth.canReview()">Pending reviewer action.</p>
            </div>
            <section class="backfill surface" *ngIf="resolution.resolution_status === 'accepted'">
              <div class="section-title"><h2>Execution backfill</h2><span>{{ backfills().length }} records · append only</span></div>
              <p class="rule">Actual contact results for the accepted plan. Start deviation over 10 min or duration difference over 5 min flags a record for re-check; normal results are archived. New records never change earlier conclusions.</p>
              <table *ngIf="backfills().length"><thead><tr><th>Window</th><th>Actual interval</th><th>Elev</th><th>Outcome</th><th>Deviation</th><th>Status</th><th>Recorded</th></tr></thead><tbody><tr *ngFor="let record of backfills()"><td class="code">#{{ record.window_id }}</td><td>{{ format(record.actual_start_at) }}<br><span>{{ format(record.actual_end_at) }}</span></td><td>{{ record.actual_elevation_peak_deg }}°</td><td><app-window-status-badge [status]="record.outcome" /></td><td>start {{ deviation(record.start_deviation_sec) }}<br><span>dur {{ deviation(record.duration_deviation_sec) }}</span></td><td><app-window-status-badge [status]="record.review_status" /></td><td>{{ record.recorded_by }}<br><span>{{ format(record.created_at) }}</span></td></tr></tbody></table>
              <div class="empty" *ngIf="!backfills().length">No execution records yet.</div>
              <form *ngIf="auth.canPlan()" [formGroup]="backfillForm" (ngSubmit)="saveBackfill(resolution)">
                <mat-form-field appearance="outline"><mat-label>Window</mat-label><mat-select formControlName="window_id" (selectionChange)="prefillBackfill()"><mat-option *ngFor="let id of resolution.window_ids" [value]="id">Window #{{ id }}</mat-option></mat-select></mat-form-field>
                <mat-form-field appearance="outline"><mat-label>Actual start</mat-label><input matInput type="datetime-local" formControlName="actual_start"></mat-form-field>
                <mat-form-field appearance="outline"><mat-label>Actual end</mat-label><input matInput type="datetime-local" formControlName="actual_end"></mat-form-field>
                <mat-form-field appearance="outline"><mat-label>Peak elevation (deg)</mat-label><input matInput type="number" formControlName="elevation"></mat-form-field>
                <mat-form-field appearance="outline"><mat-label>Outcome</mat-label><mat-select formControlName="outcome"><mat-option value="success">success</mat-option><mat-option value="failed">failed</mat-option></mat-select></mat-form-field>
                <mat-form-field appearance="outline" class="note"><mat-label>Result note</mat-label><textarea matInput rows="2" formControlName="note"></textarea></mat-form-field>
                <button mat-flat-button color="primary" type="submit" [disabled]="backfillForm.invalid || savingBackfill()"><span class="spinner" *ngIf="savingBackfill()"></span>{{ savingBackfill() ? 'Recording' : 'Record execution' }}</button>
              </form>
            </section>
          </section>
        </ng-container>
        <ng-template #noSelection><div class="empty surface">Select a conflict group to inspect evidence and suggestions.</div></ng-template>
      </div>
    </div>
  `,
  styles: [`
    .wide { width: min(1600px, 100%); }
    .detection-bar { display: grid; grid-template-columns: repeat(3, minmax(180px, 1fr)) auto; gap: 12px; align-items: start; padding: 15px; background: #e9eeea; border: 1px solid #ccd3ce; } .detection-bar button { min-height: 48px; display: flex; gap: 8px; }
    .detection-result { display: flex; flex-wrap: wrap; gap: 18px; padding: 10px 14px; background: #173f3b; color: #eef4ef; font-size: 11px; } .detection-result span { color: #bad0c7; }
    .conflict-layout { display: grid; grid-template-columns: minmax(245px, .65fr) minmax(330px, 1fr) minmax(380px, 1.15fr); gap: 16px; align-items: start; margin-top: 20px; }
    .conflict-list { max-height: 720px; overflow-y: auto; } .conflict-list > button { width: 100%; display: grid; grid-template-columns: 34px minmax(0, 1fr) auto; align-items: center; gap: 10px; padding: 13px; border: 0; border-bottom: 1px solid #e0e5e1; background: transparent; text-align: left; cursor: pointer; } .conflict-list > button:hover, .conflict-list > button.active { background: #edf3ef; } .conflict-list strong, .conflict-list small { display: block; } .conflict-list strong { font-size: 12px; } .conflict-list small { margin-top: 3px; color: #66716d; font-size: 9px; } .conflict-list .backfill-meta { color: #294f72; } .conflict-list .backfill-meta em { color: #8a352f; font-style: normal; font-weight: 800; } .type-mark { display: grid; place-items: center; width: 31px; height: 31px; background: #dfe7e2; font-size: 9px; font-weight: 900; }
    .evidence { overflow: hidden; } .panel-head { display: grid; grid-template-columns: 1fr auto; align-items: center; padding: 18px; border-bottom: 1px solid #ccd3ce; } .panel-head .eyebrow { grid-column: 1/-1; } .panel-head h2 { margin: 0; font-size: 17px; } .summary { margin: 0; padding: 16px 18px; line-height: 1.5; font-size: 13px; } dl { display: grid; grid-template-columns: repeat(4, 1fr); margin: 0; border-block: 1px solid #dfe4e0; } dl div { padding: 11px; border-right: 1px solid #dfe4e0; } dt { color: #66716d; font-size: 9px; text-transform: uppercase; } dd { margin: 3px 0 0; font-weight: 800; } table { width: 100%; border-collapse: collapse; } th, td { padding: 9px; border-bottom: 1px solid #e2e7e3; text-align: left; font-size: 10px; } th { color: #66716d; } td span { color: #66716d; } details { padding: 12px 16px; } summary { cursor: pointer; font-size: 11px; } pre { max-width: 100%; overflow: auto; font-size: 10px; white-space: pre-wrap; }
    .decision .section-title { margin-top: 0; } .workflow { margin-top: 12px; padding: 16px; } .workflow p { color: #66716d; font-size: 12px; line-height: 1.5; } .workflow form { display: grid; } .workflow mat-form-field { width: 100%; }
    .backfill { margin-top: 16px; padding: 16px; } .backfill .rule { margin: 0 0 12px; color: #66716d; font-size: 11px; line-height: 1.5; } .backfill table { width: 100%; border-collapse: collapse; margin-bottom: 12px; } .backfill th, .backfill td { padding: 8px; border-bottom: 1px solid #e2e7e3; text-align: left; font-size: 10px; } .backfill th { color: #66716d; } .backfill td span { color: #66716d; } .backfill form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 12px; } .backfill form .note, .backfill form button { grid-column: 1/-1; } .backfill form button { min-height: 44px; display: flex; gap: 8px; justify-content: center; } .backfill .empty { padding: 10px 0; color: #66716d; font-size: 11px; }
    @media (max-width: 1220px) { .conflict-layout { grid-template-columns: 280px 1fr; } .decision { grid-column: 2; } } @media (max-width: 800px) { .detection-bar { grid-template-columns: 1fr 1fr; } .conflict-layout { grid-template-columns: 1fr; } .decision { grid-column: 1; } }
  `],
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ConflictsPage implements OnInit {
  readonly auth = useAuth(); readonly detection: ConflictDetectionHook; readonly resolutions = signal<ConflictResolution[]>([]); readonly selected = signal<ConflictResolution | null>(null); readonly selectedKey = signal(''); readonly error = signal(''); readonly typeFilter = signal(''); readonly types = CONFLICT_TYPES;
  readonly backfills = signal<ContactBackfill[]>([]); readonly savingBackfill = signal(false);
  private readonly builder = new FormBuilder().nonNullable;
  readonly rangeForm = this.builder.group({ from: ['', Validators.required], to: ['', Validators.required] }); readonly reviewForm = this.builder.group({ note: ['', Validators.maxLength(500)] });
  readonly backfillForm = this.builder.group({
    window_id: [0, [Validators.required, Validators.min(1)]],
    actual_start: ['', Validators.required],
    actual_end: ['', Validators.required],
    elevation: [60, [Validators.required, Validators.min(0), Validators.max(90)]],
    outcome: ['success' as BackfillOutcome, Validators.required],
    note: ['', Validators.maxLength(500)],
  });
  constructor(private readonly api: ApiService, detection: ConflictDetectionHook, private readonly snackBar: MatSnackBar) { this.detection = detection; const from = new Date(Date.now() - 60 * 60 * 1000), to = new Date(Date.now() + 26 * 60 * 60 * 1000); this.rangeForm.setValue({ from: toLocalInput(from), to: toLocalInput(to) }); }
  ngOnInit(): void { this.load(); }
  load(preferredId = this.selected()?.id ?? 0): void { this.error.set(''); const params: Record<string, string | number> = { page_size: 100 }; if (this.typeFilter()) params['conflict_type'] = this.typeFilter(); this.api.conflicts(params).subscribe({ next: (response) => { this.resolutions.set(response.data); const next = response.data.find((item) => item.id === preferredId) ?? response.data[0] ?? null; this.select(next); }, error: (error: unknown) => this.error.set(apiErrorMessage(error)) }); }
  filterType(value: string): void { this.typeFilter.set(value); this.load(0); }
  select(resolution: ConflictResolution | null): void {
    this.selected.set(resolution);
    this.selectedKey.set(resolution?.selected_action?.action_key ?? resolution?.suggestions[0]?.action_key ?? '');
    this.backfills.set([]);
    if (resolution && resolution.resolution_status === 'accepted') {
      this.backfillForm.patchValue({ window_id: resolution.window_ids[0] ?? 0 });
      this.prefillBackfill();
      this.api.backfills(resolution.id).subscribe({ next: (response) => this.backfills.set(response.data), error: (error: unknown) => this.error.set(apiErrorMessage(error)) });
    }
  }
  prefillBackfill(): void {
    const resolution = this.selected();
    if (!resolution) return;
    const fact = resolution.evidence.window_facts.find((item) => item['id'] === this.backfillForm.controls.window_id.value);
    if (!fact) return;
    this.backfillForm.patchValue({ actual_start: toLocalInput(new Date(fact['start_at'] as string)), actual_end: toLocalInput(new Date(fact['end_at'] as string)) });
  }
  saveBackfill(resolution: ConflictResolution): void {
    if (this.backfillForm.invalid) return;
    const value = this.backfillForm.getRawValue();
    this.savingBackfill.set(true);
    this.api.createBackfill(resolution.id, {
      window_id: value.window_id,
      actual_start_at: new Date(value.actual_start).toISOString(),
      actual_end_at: new Date(value.actual_end).toISOString(),
      actual_elevation_peak_deg: value.elevation,
      outcome: value.outcome,
      result_note: value.note,
    }).pipe(finalize(() => this.savingBackfill.set(false))).subscribe({
      next: (response) => { this.snackBar.open(response.data.review_status === 'needs_review' ? 'Backfill flagged for re-check' : 'Backfill archived', 'Close', { duration: 2400 }); this.load(resolution.id); },
      error: (error: unknown) => this.error.set(apiErrorMessage(error)),
    });
  }
  detect(): void { if (this.rangeForm.invalid) return; const range = this.rangeForm.getRawValue(); this.detection.detect(new Date(range.from).toISOString(), new Date(range.to).toISOString(), () => { this.snackBar.open('Conflict scan completed', 'Close', { duration: 2200 }); this.load(0); }); }
  submitForReview(resolution: ConflictResolution): void { this.api.submitConflict(resolution.id, resolution.version).subscribe({ next: (response) => { this.snackBar.open('Submitted for review', 'Close', { duration: 2200 }); this.load(response.data.id); }, error: (error: unknown) => this.error.set(apiErrorMessage(error)) }); }
  review(resolution: ConflictResolution, decision: 'accepted' | 'rejected'): void { const key = decision === 'accepted' ? this.selectedKey() : ''; this.api.reviewConflict(resolution.id, resolution.version, decision, key, this.reviewForm.controls.note.value).pipe(finalize(() => undefined)).subscribe({ next: (response) => { this.snackBar.open(`Resolution ${decision}`, 'Close', { duration: 2200 }); this.load(response.data.id); }, error: (error: unknown) => this.error.set(apiErrorMessage(error)) }); }
  typeLabel(value: string): string { return value.replaceAll('_', ' '); } typeCode(value: ConflictType): string { return ({ station_capacity: 'SC', satellite_overlap: 'SO', band_mismatch: 'BM', duration_shortfall: 'DS', slew_buffer: 'SB' })[value]; }
  format(value: string): string { return formatUtc(value); } formatFact(value: unknown): string { return typeof value === 'string' ? formatUtc(value) : '—'; }
  deviation(seconds: number): string { return `${Math.floor(seconds / 60)}m ${seconds % 60}s`; }
}
