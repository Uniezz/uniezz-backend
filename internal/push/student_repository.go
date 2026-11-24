package push

import (
	"context"
	"errors"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type studentRepository struct {
	db database.DB
}

func newStudentRepository(db database.DB) *studentRepository {
	return &studentRepository{db: db}
}

func (r *studentRepository) firstName(ctx context.Context, userID uuid.UUID) (*string, error) {
	const query = `SELECT first_name FROM public_profiles WHERE id = $1`
	var name *string
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get student name: %w", err)
	}
	return name, nil
}
