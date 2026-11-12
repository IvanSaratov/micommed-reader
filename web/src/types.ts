export interface Metric {
  value: number | null;
  reason?: string;
  origin?: string;
}
export interface Problem {
  kind: string;
  code: string;
  date?: string;
  through?: string;
  label?: string;
  field?: string;
  index?: number;
  detail?: string;
}
export interface Source {
  source_id: string;
  date: string;
  label: string;
  sha256?: string;
  status: string;
}
export interface Period {
  from: string;
  through: string;
  included_days: number;
  counted_events: number;
  therapy_minutes: Metric;
  recalculated_ahi: Metric;
}
export interface Index {
  sources: Source[];
  period: Period;
  diagnostics: Problem[];
  partial: boolean;
  reasons: Record<string, string>;
}
export interface Sample {
  source_index?: number;
  source_date?: string;
  source_sha256?: string;
  break_before?: boolean;
  index: number;
  minute: number;
  device_time?: string;
}
export interface Point extends Sample {
  raw: number;
}
export interface Event extends Sample {
  code: number;
  label?: string;
  meaning_evidence: string;
}
export interface Field {
  field: string;
  unit: string;
  scale: number;
  meaning_evidence: string;
}
export interface Day {
  date: string;
  sha256: string;
  start_device_time: string;
  end_device_time: string;
  therapy_minutes: Metric;
  stored_ahi: Metric;
  recalculated_ahi: Metric;
  pressure: (Sample & { first_raw: number; second_raw: number })[];
  pressure_fields: [Field, Field];
  channels: (Field & { points: Point[] })[];
  events: Event[];
  diagnostics: Problem[];
  event_counts: Record<string, number>;
  fields: Record<
    string,
    { Label: string; Unit: string; Scale: number; MeaningEvidence: string }
  >;
}

export interface SleepSession {
  id: number;
  start_device_time: string;
  end_device_time: string;
  span_minutes: number;
  therapy_minutes: Metric;
  recalculated_ahi: Metric;
  event_counts: Record<string, number>;
}
export interface SessionIndex {
  gap_minutes: number;
  sessions: SleepSession[];
  issues: Problem[];
}
export type SessionView = SleepSession &
  Pick<
    Day,
    | "pressure"
    | "pressure_fields"
    | "channels"
    | "events"
    | "fields"
    | "diagnostics"
  >;
