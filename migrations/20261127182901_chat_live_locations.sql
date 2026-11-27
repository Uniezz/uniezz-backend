-- +goose Up
CREATE TABLE chat_live_locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lat DOUBLE PRECISION NOT NULL CHECK (lat BETWEEN -90 AND 90),
    lng DOUBLE PRECISION NOT NULL CHECK (lng BETWEEN -180 AND 180),
    accuracy INT CHECK (accuracy BETWEEN 0 AND 1500),
    heading INT CHECK (heading BETWEEN 1 AND 360),
    live_until TIMESTAMPTZ NOT NULL,
    moved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_chat_live_locations_message ON chat_live_locations (message_id);
CREATE INDEX idx_chat_live_locations_user_id ON chat_live_locations (user_id);
CREATE INDEX idx_chat_live_locations_live_until ON chat_live_locations (live_until);

CREATE TRIGGER set_chat_live_locations_updated_at
    BEFORE UPDATE ON chat_live_locations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TRIGGER set_chat_live_locations_updated_at ON chat_live_locations;
DROP TABLE chat_live_locations;
