package legacysynchttp

import "testing"

func TestValidateRefreshRequest(t *testing.T) {
	tests := []struct {
		name     string
		request  refreshRequest
		wantType string
		wantKey  string
		wantErr  bool
	}{
		{name: "course", request: refreshRequest{EntityType: " Course ", ExternalID: "7306"}, wantType: "legacy_refresh_course", wantKey: "legacy:course:7306"},
		{name: "full", request: refreshRequest{EntityType: "full"}, wantType: "legacy_full_reconcile", wantKey: "legacy:full"},
		{name: "missing external ID", request: refreshRequest{EntityType: "course"}, wantErr: true},
		{name: "unsafe entity", request: refreshRequest{EntityType: "course/delete", ExternalID: "7306"}, wantErr: true},
		{name: "priority too high", request: refreshRequest{EntityType: "course", ExternalID: "7306", Priority: int32Pointer(101)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateRefreshRequest(tt.request)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateRefreshRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (got.JobType != tt.wantType || got.UniqueKey != tt.wantKey) {
				t.Fatalf("validateRefreshRequest() = %+v", got)
			}
		})
	}
}

func int32Pointer(value int32) *int32 { return &value }
