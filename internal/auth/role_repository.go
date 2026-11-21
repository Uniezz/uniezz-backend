package auth

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type roleRepository struct {
	db database.DB
}

func newRoleRepository(db database.DB) *roleRepository {
	return &roleRepository{db: db}
}

func (r *roleRepository) grant(ctx context.Context, userID uuid.UUID, role Role) (bool, error) {
	const query = `
		WITH target AS (SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL),
		granted AS (
			INSERT INTO user_roles (user_id, role) SELECT id, $2 FROM target
			ON CONFLICT (user_id, role) DO NOTHING
		)
		SELECT EXISTS (SELECT 1 FROM target)`
	var found bool
	if err := r.db.Conn(ctx).QueryRow(ctx, query, userID, role).Scan(&found); err != nil {
		return false, fmt.Errorf("grant role: %w", err)
	}
	return found, nil
}

func (r *roleRepository) revoke(ctx context.Context, userID uuid.UUID, role Role) error {
	const query = `DELETE FROM user_roles WHERE user_id = $1 AND role = $2`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, userID, role); err != nil {
		return fmt.Errorf("revoke role: %w", err)
	}
	return nil
}
