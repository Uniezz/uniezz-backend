package moderation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type moderatorRow struct {
	UserID    uuid.UUID
	FirstName *string
	LastName  *string
	GrantedAt time.Time
}

type moderatorCursor struct {
	GrantedAt time.Time `json:"c"`
	UserID    uuid.UUID `json:"i"`
}

type moderatorFilter struct {
	After *moderatorCursor
	Limit int
}

type moderatorRepository struct {
	db database.DB
}

func newModeratorRepository(db database.DB) *moderatorRepository {
	return &moderatorRepository{db: db}
}

func (r *moderatorRepository) list(ctx context.Context, f moderatorFilter) ([]moderatorRow, error) {
	var args database.Args
	arg := args.Add

	where := []string{"s.role = " + arg(auth.RoleModerator)}
	if f.After != nil {
		where = append(where, "(s.created_at, s.user_id) < ("+arg(f.After.GrantedAt)+", "+arg(f.After.UserID)+")")
	}

	query := `
		SELECT s.user_id, p.first_name, p.last_name, s.created_at
		FROM staff_roles s
		JOIN public_profiles p ON p.id = s.user_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY s.created_at DESC, s.user_id DESC
		LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list moderators: %w", err)
	}
	defer rows.Close()

	var moderators []moderatorRow
	for rows.Next() {
		var row moderatorRow
		if err := rows.Scan(&row.UserID, &row.FirstName, &row.LastName, &row.GrantedAt); err != nil {
			return nil, fmt.Errorf("scan moderator: %w", err)
		}
		moderators = append(moderators, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list moderators: %w", err)
	}
	return moderators, nil
}
