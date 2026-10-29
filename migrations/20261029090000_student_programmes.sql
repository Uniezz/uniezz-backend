-- +goose Up
CREATE TABLE student_programmes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    faculty_name VARCHAR(255) NOT NULL,
    programme_name VARCHAR(255) NOT NULL,
    level TEXT CHECK (level IN ('first_cycle', 'second_cycle', 'long_cycle', 'third_cycle', 'phd_school')),
    mode_of_studies TEXT CHECK (mode_of_studies IN ('full_time', 'part_time')),
    year_of_study INT CHECK (year_of_study BETWEEN 1 AND 6),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'graduated', 'suspended', 'cancelled')),
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_student_programmes_user_id ON student_programmes (user_id);
CREATE UNIQUE INDEX uq_student_programmes_primary ON student_programmes (user_id) WHERE is_primary;

CREATE TRIGGER set_student_programmes_updated_at
BEFORE UPDATE ON student_programmes
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE student_programmes;
