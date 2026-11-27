package chat

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type pollRepository struct {
	db database.DB
}

func newPollRepository(db database.DB) *pollRepository {
	return &pollRepository{db: db}
}

func (r *pollRepository) myVote(ctx context.Context, messageID, userID uuid.UUID) ([]int, error) {
	const query = `SELECT option_id FROM chat_poll_votes WHERE message_id = $1 AND user_id = $2 ORDER BY option_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, messageID, userID)
	if err != nil {
		return nil, fmt.Errorf("get poll vote: %w", err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan poll vote: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get poll vote: %w", err)
	}
	return ids, nil
}

func (r *pollRepository) replaceVote(ctx context.Context, messageID, userID uuid.UUID, optionIDs []int) error {
	const remove = `DELETE FROM chat_poll_votes WHERE message_id = $1 AND user_id = $2`
	if _, err := r.db.Conn(ctx).Exec(ctx, remove, messageID, userID); err != nil {
		return fmt.Errorf("delete poll vote: %w", err)
	}
	if len(optionIDs) == 0 {
		return nil
	}
	const insert = `
		INSERT INTO chat_poll_votes (message_id, user_id, option_id)
		SELECT $1, $2, unnest($3::smallint[])`
	if _, err := r.db.Conn(ctx).Exec(ctx, insert, messageID, userID, optionIDs); err != nil {
		return fmt.Errorf("create poll vote: %w", err)
	}
	return nil
}

func (r *pollRepository) deleteVotes(ctx context.Context, messageID uuid.UUID) error {
	const query = `DELETE FROM chat_poll_votes WHERE message_id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, messageID); err != nil {
		return fmt.Errorf("delete poll votes: %w", err)
	}
	return nil
}

func (r *pollRepository) close(ctx context.Context, messageID uuid.UUID) (bool, error) {
	const query = `
		UPDATE chat_messages
		SET payload = jsonb_set(payload, '{closedAt}', to_jsonb(NOW()))
		WHERE id = $1 AND kind = $2 AND deleted_at IS NULL AND payload->'closedAt' = 'null'::jsonb`
	result, err := r.db.Conn(ctx).Exec(ctx, query, messageID, MessagePoll)
	if err != nil {
		return false, fmt.Errorf("close poll: %w", err)
	}
	return result.RowsAffected() > 0, nil
}

func (r *pollRepository) counts(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID]map[int]int, error) {
	const query = `
		SELECT message_id, option_id, COUNT(*)
		FROM chat_poll_votes
		WHERE message_id = ANY($1)
		GROUP BY message_id, option_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("count poll votes: %w", err)
	}
	defer rows.Close()

	counts := map[uuid.UUID]map[int]int{}
	for rows.Next() {
		var id uuid.UUID
		var option, n int
		if err := rows.Scan(&id, &option, &n); err != nil {
			return nil, fmt.Errorf("scan poll count: %w", err)
		}
		if counts[id] == nil {
			counts[id] = map[int]int{}
		}
		counts[id][option] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count poll votes: %w", err)
	}
	return counts, nil
}

func (r *pollRepository) totals(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	const query = `
		SELECT message_id, COUNT(DISTINCT user_id)
		FROM chat_poll_votes
		WHERE message_id = ANY($1)
		GROUP BY message_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("count poll voters: %w", err)
	}
	defer rows.Close()

	totals := map[uuid.UUID]int{}
	for rows.Next() {
		var id uuid.UUID
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan poll voters: %w", err)
		}
		totals[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count poll voters: %w", err)
	}
	return totals, nil
}

func (r *pollRepository) votesOf(ctx context.Context, messageIDs []uuid.UUID, userID uuid.UUID) (map[uuid.UUID][]int, error) {
	const query = `
		SELECT message_id, option_id
		FROM chat_poll_votes
		WHERE message_id = ANY($1) AND user_id = $2
		ORDER BY message_id, option_id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, messageIDs, userID)
	if err != nil {
		return nil, fmt.Errorf("list poll votes: %w", err)
	}
	defer rows.Close()

	votes := map[uuid.UUID][]int{}
	for rows.Next() {
		var id uuid.UUID
		var option int
		if err := rows.Scan(&id, &option); err != nil {
			return nil, fmt.Errorf("scan poll vote: %w", err)
		}
		votes[id] = append(votes[id], option)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list poll votes: %w", err)
	}
	return votes, nil
}

func (r *pollRepository) recentVoters(ctx context.Context, messageIDs []uuid.UUID, viewerID uuid.UUID, limit int) (map[uuid.UUID][]uuid.UUID, error) {
	const query = `
		SELECT message_id, user_id
		FROM (
			SELECT v.message_id, v.user_id,
				ROW_NUMBER() OVER (PARTITION BY v.message_id ORDER BY MAX(v.created_at) DESC, v.user_id) AS n
			FROM chat_poll_votes v
			WHERE v.message_id = ANY($1)
			  AND NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = $2 AND b.other_id = v.user_id)
			  AND NOT EXISTS (SELECT 1 FROM banned_users x WHERE x.id = v.user_id)
			GROUP BY v.message_id, v.user_id
		) ranked
		WHERE n <= $3
		ORDER BY message_id, n`
	return scanVoters(ctx, r.db, "list poll voters", query, messageIDs, viewerID, limit)
}

func scanVoters(ctx context.Context, db database.DB, what, query string, args ...any) (map[uuid.UUID][]uuid.UUID, error) {
	rows, err := db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer rows.Close()

	voters := map[uuid.UUID][]uuid.UUID{}
	for rows.Next() {
		var messageID, userID uuid.UUID
		if err := rows.Scan(&messageID, &userID); err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		voters[messageID] = append(voters[messageID], userID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return voters, nil
}
