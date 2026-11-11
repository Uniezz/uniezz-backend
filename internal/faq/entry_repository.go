package faq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type entryRow struct {
	ID        uuid.UUID
	Question  string
	Answer    string
	Status    string
	CreatedAt time.Time
}

type entryRepository struct {
	db database.DB
}

func newEntryRepository(db database.DB) *entryRepository {
	return &entryRepository{db: db}
}

func (r *entryRepository) insert(ctx context.Context, authorID uuid.UUID, question, answer, status string) (*entryRow, error) {
	row := r.db.Conn(ctx).QueryRow(ctx, `
		INSERT INTO faq_entries (author_id, question, answer, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, question, answer, status, created_at`, authorID, question, answer, status)
	return scanEntry(row)
}

func (r *entryRepository) userEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	const query = `SELECT email FROM user_emails WHERE id = $1`
	var email *string
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get user email: %w", err)
	}
	if email == nil {
		return "", nil
	}
	return *email, nil
}

func (r *entryRepository) list(ctx context.Context, authorID *uuid.UUID, status string, after *pageCursor, limit int) ([]entryRow, error) {
	args := database.Args{}
	var where []string
	if authorID != nil {
		where = append(where, "author_id = "+args.Add(*authorID))
	}
	if status != "" {
		where = append(where, "status = "+args.Add(status))
	}
	if after != nil {
		where = append(where, "(created_at, id) < ("+args.Add(after.CreatedAt)+", "+args.Add(after.ID)+")")
	}
	query := `SELECT id, question, answer, status, created_at FROM faq_entries WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY created_at DESC, id DESC LIMIT ` + args.Add(limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list faq entries: %w", err)
	}
	defer rows.Close()

	var entries []entryRow
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan faq entry: %w", err)
		}
		entries = append(entries, *entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list faq entries: %w", err)
	}
	return entries, nil
}

func scanEntry(row interface{ Scan(dest ...any) error }) (*entryRow, error) {
	var entry entryRow
	if err := row.Scan(&entry.ID, &entry.Question, &entry.Answer, &entry.Status, &entry.CreatedAt); err != nil {
		return nil, err
	}
	return &entry, nil
}
