package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AdminUserListParams struct {
	IncludeDeleted bool
}

func (q *Queries) AdminUserList(ctx context.Context, p AdminUserListParams) ([]User, error) {
	var rows pgx.Rows
	var err error
	if p.IncludeDeleted {
		rows, err = q.db.Query(ctx, `
			SELECT id, username, role, password_hash, password_version, deleted_at, created_at, updated_at
			FROM users
			ORDER BY username ASC
		`)
	} else {
		rows, err = q.db.Query(ctx, `
			SELECT id, username, role, password_hash, password_version, deleted_at, created_at, updated_at
			FROM users
			WHERE deleted_at IS NULL
			ORDER BY username ASC
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &u.PasswordVersion, &u.DeletedAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

type AdminUserCreateParams struct {
	Username     string
	Role         string
	PasswordHash string
}

func (q *Queries) AdminUserCreate(ctx context.Context, p AdminUserCreateParams) (pgtype.UUID, error) {
	if p.Username == "" || p.Role == "" || p.PasswordHash == "" {
		return pgtype.UUID{}, fmt.Errorf("missing required fields")
	}
	var id pgtype.UUID
	err := q.db.QueryRow(ctx, `
		INSERT INTO users (username, role, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id
	`, p.Username, p.Role, p.PasswordHash).Scan(&id)
	return id, err
}

func (q *Queries) AdminUserDeactivate(ctx context.Context, userID pgtype.UUID) error {
	_, err := q.db.Exec(ctx, `
		UPDATE users
		SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, userID)
	if err != nil {
		return err
	}
	// Best-effort: revoke all sessions; RequireUser also blocks deleted users.
	_, _ = q.db.Exec(ctx, `
		UPDATE auth_sessions
		SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	return nil
}

func (q *Queries) AdminUserResetPassword(ctx context.Context, userID pgtype.UUID, newPasswordHash string) error {
	if newPasswordHash == "" {
		return fmt.Errorf("password hash required")
	}

	type beginner interface {
		Begin(context.Context) (pgx.Tx, error)
	}

	db, ok := q.db.(beginner)
	if ok {
		tx, err := db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback(ctx)

		tag, err := tx.Exec(ctx, `
			UPDATE users
			SET password_hash = $2,
			    password_version = password_version + 1,
			    updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL
		`, userID, newPasswordHash)
		if err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("user not found or deleted")
		}

		_, err = tx.Exec(ctx, `
			UPDATE auth_sessions
			SET revoked_at = now()
			WHERE user_id = $1 AND revoked_at IS NULL
		`, userID)
		if err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}

		return tx.Commit(ctx)
	}

	tag, err := q.db.Exec(ctx, `
		UPDATE users
		SET password_hash = $2,
		    password_version = password_version + 1,
		    updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, userID, newPasswordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user not found or deleted")
	}

	_, err = q.db.Exec(ctx, `
		UPDATE auth_sessions
		SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	return err
}
