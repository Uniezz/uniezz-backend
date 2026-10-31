-- +goose Up
CREATE TABLE feed_posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id UUID NOT NULL REFERENCES users(id),
    university_id VARCHAR(32) NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('announcement', 'event', 'question')),
    title VARCHAR(150) NOT NULL CHECK (char_length(title) >= 3),
    content TEXT NOT NULL CHECK (char_length(content) BETWEEN 10 AND 5000),
    event_date TIMESTAMPTZ CHECK (type = 'event' OR event_date IS NULL),
    tags TEXT[] NOT NULL DEFAULT '{}' CHECK (cardinality(tags) <= 10),
    likes_count INT NOT NULL DEFAULT 0 CHECK (likes_count >= 0),
    comments_count INT NOT NULL DEFAULT 0 CHECK (comments_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    edited_at TIMESTAMPTZ
);

CREATE INDEX idx_feed_posts_created_at_id ON feed_posts (created_at DESC, id DESC);
CREATE INDEX idx_feed_posts_university_created_at_id ON feed_posts (university_id, created_at DESC, id DESC);
CREATE INDEX idx_feed_posts_type_created_at_id ON feed_posts (type, created_at DESC, id DESC);
CREATE INDEX idx_feed_posts_author_id ON feed_posts (author_id);

CREATE TRIGGER set_feed_posts_updated_at
BEFORE UPDATE ON feed_posts
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE feed_post_media (
    post_id UUID NOT NULL REFERENCES feed_posts(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL CHECK (position BETWEEN 0 AND 9),
    media_id UUID NOT NULL UNIQUE REFERENCES media(id),
    PRIMARY KEY (post_id, position)
);

CREATE TABLE feed_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES feed_posts(id) ON DELETE CASCADE,
    author_id UUID REFERENCES users(id),
    parent_id UUID,
    reply_to_comment_id UUID,
    thread_id UUID NOT NULL GENERATED ALWAYS AS (COALESCE(parent_id, id)) STORED,
    is_root BOOLEAN NOT NULL GENERATED ALWAYS AS (parent_id IS NULL) STORED,
    parent_is_root BOOLEAN GENERATED ALWAYS AS (CASE WHEN parent_id IS NOT NULL THEN TRUE END) STORED,
    content VARCHAR(1000) CHECK (char_length(content) >= 1),
    likes_count INT NOT NULL DEFAULT 0 CHECK (likes_count >= 0),
    replies_count INT NOT NULL DEFAULT 0 CHECK (replies_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    edited_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT feed_comments_tombstone_check CHECK ((deleted_at IS NULL) = (content IS NOT NULL)),
    CONSTRAINT feed_comments_author_check CHECK (author_id IS NOT NULL OR deleted_at IS NOT NULL),
    CONSTRAINT feed_comments_tombstone_fields_check CHECK (deleted_at IS NULL OR (author_id IS NULL AND likes_count = 0 AND parent_id IS NULL)),
    CONSTRAINT feed_comments_reply_to_check CHECK (parent_id IS NOT NULL OR reply_to_comment_id IS NULL),
    CONSTRAINT feed_comments_replies_check CHECK (parent_id IS NULL OR replies_count = 0),
    CONSTRAINT uq_feed_comments_post_id_root UNIQUE (post_id, id, is_root),
    CONSTRAINT uq_feed_comments_thread_id UNIQUE (thread_id, id),
    CONSTRAINT feed_comments_parent_fkey FOREIGN KEY (post_id, parent_id, parent_is_root)
        REFERENCES feed_comments (post_id, id, is_root),
    CONSTRAINT feed_comments_reply_to_fkey FOREIGN KEY (parent_id, reply_to_comment_id)
        REFERENCES feed_comments (thread_id, id) ON DELETE SET NULL (reply_to_comment_id)
);

CREATE INDEX idx_feed_comments_post_parent_created_at ON feed_comments (post_id, parent_id, created_at, id);
CREATE INDEX idx_feed_comments_parent_created_at ON feed_comments (parent_id, created_at, id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_feed_comments_reply_to ON feed_comments (reply_to_comment_id) WHERE reply_to_comment_id IS NOT NULL;
CREATE INDEX idx_feed_comments_author_id ON feed_comments (author_id);

CREATE TRIGGER set_feed_comments_updated_at
BEFORE UPDATE ON feed_comments
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE feed_post_likes (
    post_id UUID NOT NULL REFERENCES feed_posts(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (post_id, user_id)
);

CREATE INDEX idx_feed_post_likes_user_id ON feed_post_likes (user_id);

CREATE TABLE feed_comment_likes (
    comment_id UUID NOT NULL REFERENCES feed_comments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (comment_id, user_id)
);

CREATE INDEX idx_feed_comment_likes_user_id ON feed_comment_likes (user_id);

CREATE TABLE feed_request_keys (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    operation TEXT NOT NULL CHECK (operation IN ('create_post', 'create_comment')),
    key VARCHAR(255) NOT NULL,
    fingerprint BYTEA NOT NULL,
    resource_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, operation, key)
);

CREATE INDEX idx_feed_request_keys_created_at ON feed_request_keys (created_at);

-- +goose Down
DROP TABLE feed_request_keys;
DROP TABLE feed_comment_likes;
DROP TABLE feed_post_likes;
DROP TABLE feed_comments;
DROP TABLE feed_post_media;
DROP TABLE feed_posts;
