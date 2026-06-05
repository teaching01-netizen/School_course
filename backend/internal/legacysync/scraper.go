package legacysync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
)

type Scraper struct {
	client *Client
	syncer *Syncer
	pool   *pgxpool.Pool
	q      *sqldb.Queries
	log    *slog.Logger
	loc    *time.Location
}

func NewScraper(client *Client, pool *pgxpool.Pool, q *sqldb.Queries, log *slog.Logger, loc *time.Location) *Scraper {
	return &Scraper{
		client: client,
		syncer: NewSyncer(pool, q, log, loc),
		pool:   pool,
		q:      q,
		log:    log,
		loc:    loc,
	}
}

type ScrapeResult struct {
	SessionsCreated int
	SyncedAt        time.Time
}

func (s *Scraper) SyncCourse(ctx context.Context, courseID pgtype.UUID, legacyCourseID string) (*ScrapeResult, error) {
	if err := s.client.Login(); err != nil {
		return nil, fmt.Errorf("legacy login: %w", err)
	}

	htmlContent, err := s.client.FetchSchedulePage(legacyCourseID)
	if err != nil {
		return nil, fmt.Errorf("fetch schedule: %w", err)
	}

	parsed, err := ParseScheduleTable(htmlContent)
	if err != nil {
		return nil, fmt.Errorf("parse schedule: %w", err)
	}

	s.log.Info("parsed legacy schedule rows", "count", len(parsed), "course_id", legacyCourseID)

	rooms, err := s.fetchRooms(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch rooms: %w", err)
	}

	result, err := s.syncer.SyncCourse(ctx, courseID, parsed, rooms)
	if err != nil {
		return nil, fmt.Errorf("sync course: %w", err)
	}

	return &ScrapeResult{
		SessionsCreated: result.SessionsCreated,
		SyncedAt:        result.SyncedAt,
	}, nil
}

func (s *Scraper) fetchRooms(ctx context.Context) ([]Room, error) {
	dbRooms, err := s.q.RoomList(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Room, 0, len(dbRooms))
	for _, r := range dbRooms {
		uid, err := uuidToString(r.ID)
		if err != nil {
			s.log.Warn("skipping room with invalid UUID", "error", err)
			continue
		}
		out = append(out, Room{ID: uid, Name: r.Name})
	}
	return out, nil
}

func uuidToString(u pgtype.UUID) (string, error) {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16]), nil
}
