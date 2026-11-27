-- +goose Up
CREATE TABLE chat_link_previews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    url_hash BYTEA NOT NULL CHECK (octet_length(url_hash) = 32),
    url TEXT NOT NULL CHECK (char_length(url) <= 2048),
    status TEXT NOT NULL CHECK (status IN ('ok', 'failed')),
    title TEXT CHECK (char_length(title) <= 200),
    description TEXT CHECK (char_length(description) <= 300),
    site_name TEXT CHECK (char_length(site_name) <= 100),
    media_id UUID REFERENCES media(id) ON DELETE SET NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_chat_link_previews_url_hash ON chat_link_previews (url_hash);
CREATE INDEX idx_chat_link_previews_fetched_at ON chat_link_previews (fetched_at);
CREATE INDEX idx_chat_link_previews_media_id ON chat_link_previews (media_id);

CREATE TRIGGER set_chat_link_previews_updated_at
    BEFORE UPDATE ON chat_link_previews
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE chat_message_links (
    message_id UUID PRIMARY KEY REFERENCES chat_messages(id) ON DELETE CASCADE,
    url_hash BYTEA NOT NULL CHECK (octet_length(url_hash) = 32),
    url TEXT NOT NULL CHECK (char_length(url) <= 2048),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_message_links_url_hash ON chat_message_links (url_hash);

-- +goose Down
DROP TABLE chat_message_links;
DROP TRIGGER set_chat_link_previews_updated_at ON chat_link_previews;
DROP TABLE chat_link_previews;
