export interface SessionSnapshotV1 {
  schema_version: 1;
  session_id: string;
  session_version: number;
  start_at: string; // UTC ISO-8601
  end_at: string; // UTC ISO-8601
  timezone: string;
  course: {
    id: string;
    code: string;
    name: string;
  };
  room: {
    id: string | null;
    name: string | null;
  };
  teacher: {
    id: string | null;
    name: string | null;
  };
  series_id: string | null;
  occurrence_status: string;
  captured_at: string;
}

export interface SnapshotEntity {
  id: string;
  code: string;
  name: string;
}

export interface NullableSnapshotEntity {
  id: string | null;
  name: string | null;
}
