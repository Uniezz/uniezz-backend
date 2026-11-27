-- +goose Up
CREATE TABLE programmes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    university_id VARCHAR(32) NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    faculty_id UUID REFERENCES university_faculties(id) ON DELETE SET NULL,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 300),
    level TEXT NOT NULL CHECK (level IN ('first_cycle', 'second_cycle', 'long_cycle')),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_programmes_updated_at
BEFORE UPDATE ON programmes
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE UNIQUE INDEX uq_programmes_university_id_level_name ON programmes (university_id, level, public.search_text(name));
CREATE INDEX idx_programmes_university_id_name ON programmes (university_id, name, id) WHERE active;
CREATE INDEX idx_programmes_faculty_id ON programmes (faculty_id);
CREATE INDEX idx_programmes_name_search ON programmes USING gin (public.search_text(name) public.gin_trgm_ops) WHERE active;

-- +goose Down
DROP TABLE programmes;
