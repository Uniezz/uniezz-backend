package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type universityRepository struct {
	db database.DB
}

func newUniversityRepository(db database.DB) *universityRepository {
	return &universityRepository{db: db}
}

type universityRow struct {
	ID                 UniversityID
	Name               string
	AuthType           AuthType
	Position           int
	RadonInstitutionID *uuid.UUID
	Domains            []string
	Faculties          []facultyRow
}

type facultyRow struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
}

type universityFields struct {
	ID                 UniversityID
	Name               string
	AuthType           AuthType
	Position           int
	RadonInstitutionID *uuid.UUID
	Domains            []string
	Faculties          []facultyFields
}

type facultyFields struct {
	ID        *uuid.UUID
	Name      string
	Address   string
	Latitude  float64
	Longitude float64
	Position  int
}

const universityColumns = `u.id, u.name, u.auth_type, u.position, u.radon_institution_id,
	COALESCE((SELECT array_agg(d.domain ORDER BY d.domain) FROM university_domains d WHERE d.university_id = u.id), '{}'),
	COALESCE((
		SELECT json_agg(json_build_object('id', f.id, 'name', f.name, 'address', f.address, 'latitude', f.latitude, 'longitude', f.longitude) ORDER BY f.position)
		FROM university_faculties f WHERE f.university_id = u.id
	), '[]')`

func scanUniversity(row pgx.Row) (universityRow, error) {
	var u universityRow
	err := row.Scan(&u.ID, &u.Name, &u.AuthType, &u.Position, &u.RadonInstitutionID, &u.Domains, &u.Faculties)
	return u, err
}

func (r *universityRepository) list(ctx context.Context) ([]universityRow, error) {
	const query = `SELECT ` + universityColumns + ` FROM universities u ORDER BY u.position, u.id`
	rows, err := r.db.Conn(ctx).Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list universities: %w", err)
	}
	defer rows.Close()

	var items []universityRow
	for rows.Next() {
		u, err := scanUniversity(rows)
		if err != nil {
			return nil, fmt.Errorf("scan university: %w", err)
		}
		items = append(items, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list universities: %w", err)
	}
	return items, nil
}

func (r *universityRepository) get(ctx context.Context, id UniversityID) (universityRow, error) {
	const query = `SELECT ` + universityColumns + ` FROM universities u WHERE u.id = $1`
	u, err := scanUniversity(r.db.Conn(ctx).QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return universityRow{}, ErrUniversityNotFound
	}
	if err != nil {
		return universityRow{}, fmt.Errorf("get university: %w", err)
	}
	return u, nil
}

func (r *universityRepository) create(ctx context.Context, f universityFields) error {
	const query = `
		INSERT INTO universities (id, name, auth_type, position, radon_institution_id)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.Conn(ctx).Exec(ctx, query, f.ID, f.Name, f.AuthType, f.Position, f.RadonInstitutionID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return ErrUniversityExists
	}
	if err != nil {
		return fmt.Errorf("create university: %w", err)
	}
	return nil
}

func (r *universityRepository) update(ctx context.Context, f universityFields) error {
	const query = `
		UPDATE universities SET name = $2, auth_type = $3, position = $4, radon_institution_id = $5
		WHERE id = $1`
	tag, err := r.db.Conn(ctx).Exec(ctx, query, f.ID, f.Name, f.AuthType, f.Position, f.RadonInstitutionID)
	if err != nil {
		return fmt.Errorf("update university: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUniversityNotFound
	}
	return nil
}

func (r *universityRepository) clearDomains(ctx context.Context, id UniversityID) error {
	const query = `DELETE FROM university_domains WHERE university_id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, id); err != nil {
		return fmt.Errorf("clear university domains: %w", err)
	}
	return nil
}

func (r *universityRepository) addDomains(ctx context.Context, id UniversityID, domains []string) error {
	const query = `
		INSERT INTO university_domains (university_id, domain)
		SELECT $1, d FROM unnest($2::text[]) AS d`
	_, err := r.db.Conn(ctx).Exec(ctx, query, id, domains)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return ErrDomainTaken
	}
	if err != nil {
		return fmt.Errorf("add university domains: %w", err)
	}
	return nil
}

func (r *universityRepository) deleteFacultiesExcept(ctx context.Context, id UniversityID, keep []uuid.UUID) error {
	const query = `DELETE FROM university_faculties WHERE university_id = $1 AND NOT (id = ANY($2))`
	_, err := r.db.Conn(ctx).Exec(ctx, query, id, keep)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return ErrFacultyInUse
	}
	if err != nil {
		return fmt.Errorf("delete university faculties: %w", err)
	}
	return nil
}

func (r *universityRepository) updateFaculties(ctx context.Context, id UniversityID, faculties []facultyFields) (int64, error) {
	var ids []uuid.UUID
	var names, addresses []string
	var latitudes, longitudes []float64
	var positions []int32
	for _, f := range faculties {
		if f.ID == nil {
			continue
		}
		ids = append(ids, *f.ID)
		names = append(names, f.Name)
		addresses = append(addresses, f.Address)
		latitudes = append(latitudes, f.Latitude)
		longitudes = append(longitudes, f.Longitude)
		positions = append(positions, int32(f.Position))
	}
	const query = `
		UPDATE university_faculties AS uf SET
			name = f.name, address = f.address, latitude = f.latitude, longitude = f.longitude, position = f.position
		FROM unnest($2::uuid[], $3::text[], $4::text[], $5::float8[], $6::float8[], $7::int[])
			AS f(id, name, address, latitude, longitude, position)
		WHERE uf.id = f.id AND uf.university_id = $1`
	tag, err := r.db.Conn(ctx).Exec(ctx, query, id, ids, names, addresses, latitudes, longitudes, positions)
	if err != nil {
		return 0, fmt.Errorf("update university faculties: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *universityRepository) addFaculties(ctx context.Context, id UniversityID, faculties []facultyFields) error {
	var names, addresses []string
	var latitudes, longitudes []float64
	var positions []int32
	for _, f := range faculties {
		if f.ID != nil {
			continue
		}
		names = append(names, f.Name)
		addresses = append(addresses, f.Address)
		latitudes = append(latitudes, f.Latitude)
		longitudes = append(longitudes, f.Longitude)
		positions = append(positions, int32(f.Position))
	}
	const query = `
		INSERT INTO university_faculties (university_id, name, address, latitude, longitude, position)
		SELECT $1, f.name, f.address, f.latitude, f.longitude, f.position
		FROM unnest($2::text[], $3::text[], $4::float8[], $5::float8[], $6::int[])
			AS f(name, address, latitude, longitude, position)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, id, names, addresses, latitudes, longitudes, positions); err != nil {
		return fmt.Errorf("add university faculties: %w", err)
	}
	return nil
}
