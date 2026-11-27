package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type previewStatus string

const (
	previewOK     previewStatus = "ok"
	previewFailed previewStatus = "failed"
)

type messageLinkRow struct {
	MessageID      uuid.UUID
	ConversationID uuid.UUID
	SenderID       *uuid.UUID
	URLHash        []byte
	URL            string
}

type previewFields struct {
	URLHash     []byte
	URL         string
	Status      previewStatus
	Title       *string
	Description *string
	SiteName    *string
	MediaID     *uuid.UUID
}

type previewRow struct {
	MessageID   uuid.UUID
	URL         string
	Title       *string
	Description *string
	SiteName    *string
	Image       *media.Ref
}

type linkPreviewRepository struct {
	db database.DB
}

func newLinkPreviewRepository(db database.DB) *linkPreviewRepository {
	return &linkPreviewRepository{db: db}
}

func (r *linkPreviewRepository) createLink(ctx context.Context, messageID uuid.UUID, hash []byte, url string) error {
	const query = `INSERT INTO chat_message_links (message_id, url_hash, url) VALUES ($1, $2, $3)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, messageID, hash, url); err != nil {
		return fmt.Errorf("create message link: %w", err)
	}
	return nil
}

func (r *linkPreviewRepository) deleteLink(ctx context.Context, messageID uuid.UUID) error {
	const query = `DELETE FROM chat_message_links WHERE message_id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, messageID); err != nil {
		return fmt.Errorf("delete message link: %w", err)
	}
	return nil
}

func (r *linkPreviewRepository) getLink(ctx context.Context, messageID uuid.UUID) (*messageLinkRow, error) {
	const query = `
		SELECT l.message_id, msg.conversation_id, msg.sender_id, l.url_hash, l.url
		FROM chat_message_links l
		JOIN chat_messages msg ON msg.id = l.message_id
		WHERE l.message_id = $1 AND msg.deleted_at IS NULL`
	var l messageLinkRow
	err := r.db.Conn(ctx).QueryRow(ctx, query, messageID).Scan(&l.MessageID, &l.ConversationID, &l.SenderID, &l.URLHash, &l.URL)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message link: %w", err)
	}
	return &l, nil
}

func (r *linkPreviewRepository) fresh(ctx context.Context, hash []byte, okFor, failedFor time.Duration) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM chat_link_previews
			WHERE url_hash = $1
			  AND ((status = $2 AND fetched_at > NOW() - $3::interval)
			    OR (status = $4 AND fetched_at > NOW() - $5::interval)))`
	var found bool
	if err := r.db.Conn(ctx).QueryRow(ctx, query, hash, previewOK, okFor, previewFailed, failedFor).Scan(&found); err != nil {
		return false, fmt.Errorf("check link preview: %w", err)
	}
	return found, nil
}

func (r *linkPreviewRepository) replace(ctx context.Context, f previewFields) (*uuid.UUID, error) {
	const query = `
		WITH old AS (SELECT media_id FROM chat_link_previews WHERE url_hash = $1)
		INSERT INTO chat_link_previews (url_hash, url, status, title, description, site_name, media_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (url_hash) DO UPDATE
		SET url = EXCLUDED.url, status = EXCLUDED.status, title = EXCLUDED.title, description = EXCLUDED.description,
			site_name = EXCLUDED.site_name, media_id = EXCLUDED.media_id, fetched_at = NOW()
		RETURNING (SELECT media_id FROM old)`
	var previous *uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, f.URLHash, f.URL, f.Status, f.Title, f.Description, f.SiteName, f.MediaID).Scan(&previous)
	if err != nil {
		return nil, fmt.Errorf("replace link preview: %w", err)
	}
	return previous, nil
}

func (r *linkPreviewRepository) markFailed(ctx context.Context, f previewFields) error {
	const query = `
		INSERT INTO chat_link_previews (url_hash, url, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (url_hash) DO UPDATE SET fetched_at = NOW()
		WHERE chat_link_previews.status = EXCLUDED.status`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, f.URLHash, f.URL, previewFailed); err != nil {
		return fmt.Errorf("mark link preview failed: %w", err)
	}
	return nil
}

func (r *linkPreviewRepository) previews(ctx context.Context, messageIDs []uuid.UUID) ([]previewRow, error) {
	const query = `
		SELECT l.message_id, p.url, p.title, p.description, p.site_name, m.kind, m.object_key
		FROM chat_message_links l
		JOIN chat_link_previews p ON p.url_hash = l.url_hash AND p.status = $2
		LEFT JOIN ready_media m ON m.id = p.media_id
		WHERE l.message_id = ANY($1)`
	rows, err := r.db.Conn(ctx).Query(ctx, query, messageIDs, previewOK)
	if err != nil {
		return nil, fmt.Errorf("list link previews: %w", err)
	}
	defer rows.Close()

	var previews []previewRow
	for rows.Next() {
		var p previewRow
		var kind, objectKey *string
		if err := rows.Scan(&p.MessageID, &p.URL, &p.Title, &p.Description, &p.SiteName, &kind, &objectKey); err != nil {
			return nil, fmt.Errorf("scan link preview: %w", err)
		}
		p.Image = media.RefOf(kind, objectKey)
		previews = append(previews, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list link previews: %w", err)
	}
	return previews, nil
}

func (r *linkPreviewRepository) prune(ctx context.Context, okFor, failedFor time.Duration) ([]uuid.UUID, error) {
	const query = `
		DELETE FROM chat_link_previews p
		WHERE (p.status = $1 AND p.fetched_at < NOW() - $2::interval)
		   OR (p.status = $3 AND p.fetched_at < NOW() - $4::interval
		       AND NOT EXISTS (
		           SELECT 1 FROM chat_message_links l
		           JOIN chat_messages msg ON msg.id = l.message_id
		           WHERE l.url_hash = p.url_hash AND msg.deleted_at IS NULL))
		RETURNING p.media_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, previewFailed, failedFor, previewOK, okFor)
	if err != nil {
		return nil, fmt.Errorf("prune link previews: %w", err)
	}
	defer rows.Close()

	var mediaIDs []uuid.UUID
	for rows.Next() {
		var id *uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan pruned link preview: %w", err)
		}
		if id != nil {
			mediaIDs = append(mediaIDs, *id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("prune link previews: %w", err)
	}
	return mediaIDs, nil
}
