-- +goose Up
CREATE TABLE courses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id UUID REFERENCES users(id) ON DELETE SET NULL,
    name VARCHAR(150) NOT NULL CHECK (char_length(name) BETWEEN 3 AND 150),
    description TEXT NOT NULL CHECK (char_length(description) BETWEEN 1 AND 5000),
    difficulty SMALLINT NOT NULL CHECK (difficulty BETWEEN 1 AND 5),
    university_id VARCHAR(32) NOT NULL,
    faculty VARCHAR(150) NOT NULL CHECK (char_length(faculty) BETWEEN 3 AND 150),
    subject VARCHAR(150) NOT NULL CHECK (char_length(subject) BETWEEN 3 AND 150),
    semester SMALLINT NOT NULL CHECK (semester BETWEEN 1 AND 10),
    rating_count INT NOT NULL DEFAULT 0 CHECK (rating_count >= 0),
    rating_sum INT NOT NULL DEFAULT 0 CHECK (rating_sum >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_courses_updated_at
BEFORE UPDATE ON courses
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_courses_created_at ON courses (created_at DESC, id DESC);
CREATE INDEX idx_courses_university_id_faculty_subject_semester
    ON courses (university_id, faculty, subject, semester, created_at DESC, id DESC);
CREATE INDEX idx_courses_author_id ON courses (author_id);
CREATE INDEX idx_courses_subject_search ON courses USING gin (public.search_text(subject) public.gin_trgm_ops);

CREATE TABLE course_ratings (
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    PRIMARY KEY (course_id, user_id)
);

CREATE INDEX idx_course_ratings_user_id ON course_ratings (user_id);

CREATE TABLE course_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id UUID NOT NULL REFERENCES courses(id),
    media_id UUID NOT NULL REFERENCES media(id),
    author_id UUID NOT NULL REFERENCES users(id),
    title VARCHAR(150) NOT NULL CHECK (char_length(title) BETWEEN 3 AND 150),
    file_type TEXT NOT NULL CHECK (file_type IN ('exam', 'colloquium', 'lab', 'notes')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_course_files_media_id ON course_files (media_id);
CREATE INDEX idx_course_files_created_at ON course_files (created_at DESC, id DESC);
CREATE INDEX idx_course_files_course_id_created_at ON course_files (course_id, created_at DESC, id DESC);
CREATE INDEX idx_course_files_author_id_created_at ON course_files (author_id, created_at DESC, id DESC);
CREATE INDEX idx_course_files_title_search ON course_files USING gin (public.search_text(title) public.gin_trgm_ops);

CREATE TABLE course_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id UUID NOT NULL REFERENCES courses(id),
    author_id UUID NOT NULL REFERENCES users(id),
    content TEXT NOT NULL CHECK (char_length(content) BETWEEN 1 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_course_comments_course_id_created_at ON course_comments (course_id, created_at, id);
CREATE INDEX idx_course_comments_author_id ON course_comments (author_id);

CREATE TABLE professors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id UUID NOT NULL,
    radon_id TEXT NOT NULL,
    name VARCHAR(200) NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    degree VARCHAR(100) CHECK (char_length(degree) BETWEEN 1 AND 100),
    university_id VARCHAR(32) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    rating_count INT NOT NULL DEFAULT 0 CHECK (rating_count >= 0),
    rating_sum INT NOT NULL DEFAULT 0 CHECK (rating_sum >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_professors_updated_at
BEFORE UPDATE ON professors
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE UNIQUE INDEX uq_professors_external_id ON professors (external_id);
CREATE INDEX idx_professors_name ON professors (name, id) WHERE active;
CREATE INDEX idx_professors_university_id_name ON professors (university_id, name, id) WHERE active;

CREATE TABLE professor_ratings (
    professor_id UUID NOT NULL REFERENCES professors(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    PRIMARY KEY (professor_id, user_id)
);

CREATE INDEX idx_professor_ratings_user_id ON professor_ratings (user_id);

-- +goose Down
DROP TABLE professor_ratings;
DROP TABLE professors;
DROP TABLE course_comments;
DROP TABLE course_files;
DROP TABLE course_ratings;
DROP TABLE courses;
