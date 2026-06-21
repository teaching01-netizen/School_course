package pg

import (
	"strings"
	"testing"
)

func TestIsTransactionPoolerURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "Supabase transaction pooler", url: "postgres://user:pass@project.pooler.supabase.com:6543/postgres", want: true},
		{name: "generic transaction pooler port", url: "postgres://user:pass@db.example.com:6543/postgres", want: true},
		{name: "Supabase session pooler", url: "postgres://user:pass@project.pooler.supabase.com:5432/postgres", want: false},
		{name: "direct PostgreSQL", url: "postgres://user:pass@db.example.com:5432/postgres", want: false},
		{name: "local PostgreSQL", url: "postgres://localhost:5432/warwick", want: false},
		{name: "pooler text in password", url: "postgres://user:pooler.supabase.com@db.example.com:5432/postgres", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTransactionPoolerURL(tt.url); got != tt.want {
				t.Fatalf("IsTransactionPoolerURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestResolveRealtimeDatabaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		primary              string
		override             string
		primaryIsTransaction bool
		want                 string
		wantErr              string
	}{
		{
			name:    "direct primary is reused",
			primary: "postgres://db.example.com:5432/app",
			want:    "postgres://db.example.com:5432/app",
		},
		{
			name:                 "marked transaction primary requires override",
			primary:              "postgres://db.example.com:6432/app",
			primaryIsTransaction: true,
			wantErr:              "REALTIME_DATABASE_URL",
		},
		{
			name:    "known transaction primary requires override",
			primary: "postgres://db.example.com:6543/app",
			wantErr: "REALTIME_DATABASE_URL",
		},
		{
			name:                 "direct override is accepted",
			primary:              "postgres://db.example.com:6543/app",
			override:             "postgres://db.example.com:5432/app",
			primaryIsTransaction: true,
			want:                 "postgres://db.example.com:5432/app",
		},
		{
			name:     "transaction override is rejected",
			primary:  "postgres://db.example.com:6543/app",
			override: "postgres://db.example.com:6543/app",
			wantErr:  "session-capable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveRealtimeDatabaseURL(tt.primary, tt.override, tt.primaryIsTransaction)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ResolveRealtimeDatabaseURL() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRealtimeDatabaseURL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveRealtimeDatabaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
