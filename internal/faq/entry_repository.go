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
	Language  string
	Status    string
	CreatedAt time.Time
}

type entryFields struct {
	AuthorID uuid.UUID
	Language string
	Texts    map[string]EntryText
	Status   string
}

type entryFilter struct {
	Language string
	AuthorID *uuid.UUID
	Status   string
	After    *pageCursor
	Limit    int
}

type entryRepository struct {
	db database.DB
}

func newEntryRepository(db database.DB) *entryRepository {
	return &entryRepository{db: db}
}

func (r *entryRepository) insert(ctx context.Context, f entryFields) (*entryRow, error) {
	row := entryRow{
		Question: f.Texts[f.Language].Question,
		Answer:   f.Texts[f.Language].Answer,
		Language: f.Language,
		Status:   f.Status,
	}
	err := r.db.Conn(ctx).QueryRow(ctx, `
		INSERT INTO faq_entries (author_id, language, texts, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`, f.AuthorID, f.Language, f.Texts, f.Status).Scan(&row.ID, &row.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert faq entry: %w", err)
	}
	return &row, nil
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

func (r *entryRepository) userLanguage(ctx context.Context, userID uuid.UUID) (string, error) {
	var lang string
	err := r.db.Conn(ctx).QueryRow(ctx, `SELECT language FROM users WHERE id = $1`, userID).Scan(&lang)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get user language: %w", err)
	}
	return lang, nil
}

func (r *entryRepository) list(ctx context.Context, f entryFilter) ([]entryRow, error) {
	args := database.Args{}
	lang := args.Add(f.Language) + "::text"
	// l.lang: the requested language when the entry has it, otherwise the author's.
	query := `
		SELECT e.id, e.texts -> l.lang ->> 'question', e.texts -> l.lang ->> 'answer', l.lang, e.status, e.created_at
		FROM faq_entries e
		CROSS JOIN LATERAL (
			SELECT CASE WHEN e.texts ? ` + lang + ` THEN ` + lang + ` ELSE e.language END AS lang
		) l`
	where := []string{"TRUE"}
	if f.AuthorID != nil {
		where = append(where, "e.author_id = "+args.Add(*f.AuthorID))
	}
	if f.Status != "" {
		where = append(where, "e.status = "+args.Add(f.Status))
	}
	if f.After != nil {
		where = append(where, "(e.created_at, e.id) < ("+args.Add(f.After.CreatedAt)+", "+args.Add(f.After.ID)+")")
	}
	query += ` WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY e.created_at DESC, e.id DESC LIMIT ` + args.Add(f.Limit)
	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list faq entries: %w", err)
	}
	defer rows.Close()
	var entries []entryRow
	for rows.Next() {
		var entry entryRow
		if err := rows.Scan(&entry.ID, &entry.Question, &entry.Answer, &entry.Language, &entry.Status, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan faq entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list faq entries: %w", err)
	}
	return entries, nil
}

// publishProposal publishes a proposal with its text in every language; on a published entry it replaces the texts.
func (r *entryRepository) publishProposal(ctx context.Context, id uuid.UUID, texts map[string]EntryText) (*entryRow, uuid.UUID, error) {
	const query = `
		UPDATE faq_entries SET status = $2, texts = $4
		WHERE id = $1 AND status IN ($2, $3)
		RETURNING id, texts -> language ->> 'question', texts -> language ->> 'answer', language, status, created_at, author_id`
	var entry entryRow
	var authorID uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, id, statusPublished, statusProposed, texts).
		Scan(&entry.ID, &entry.Question, &entry.Answer, &entry.Language, &entry.Status, &entry.CreatedAt, &authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, uuid.Nil, ErrNotFound
	}
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("publish faq proposal: %w", err)
	}
	return &entry, authorID, nil
}
