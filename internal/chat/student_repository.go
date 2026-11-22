package chat

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
)

type studentRow struct {
	ID           uuid.UUID
	FirstName    *string
	LastName     *string
	UniversityID string
	Avatar       *media.Ref
}

type studentRepository struct {
	db database.DB
}

func newStudentRepository(db database.DB) *studentRepository {
	return &studentRepository{db: db}
}

func (r *studentRepository) getMany(ctx context.Context, ids []uuid.UUID) ([]studentRow, error) {
	const query = `
		SELECT u.id, u.first_name, u.last_name, u.university_id, a.kind, a.object_key
		FROM public_profiles u
		LEFT JOIN ready_media a ON a.id = u.avatar_media_id
		WHERE u.id = ANY($1)`
	rows, err := r.db.Conn(ctx).Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("list students: %w", err)
	}
	defer rows.Close()

	var students []studentRow
	for rows.Next() {
		var s studentRow
		var kind, key *string
		if err := rows.Scan(&s.ID, &s.FirstName, &s.LastName, &s.UniversityID, &kind, &key); err != nil {
			return nil, fmt.Errorf("scan student: %w", err)
		}
		s.Avatar = media.RefOf(kind, key)
		students = append(students, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list students: %w", err)
	}
	return students, nil
}
