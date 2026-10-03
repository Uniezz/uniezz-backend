-- +goose Up

CREATE TYPE PLATFORM AS ENUM ('mobile', 'web');
CREATE TYPE PROVIDER AS ENUM ('umcs', 'um');

CREATE TABLE oauth_states (
  ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  requestToken TEXT NOT NULL,
  requestSecret Text NOT NULL,
  platform PLATFORM NOT NULL,
  provider PROVIDER NOT NULL,
  expiresAt TIMESTAMP NOT NULL DEFAULT (NOW() + INTERVAL '5 minutes')
);

CREATE INDEX idx_oauth_states_lookup
ON oauth_states (requestToken, expiresAt DESC);

-- +goose Down
DROP TABLE oauth_states;
DROP TYPE PLATFORM;
DROP TYPE PROVIDER;
DROP INDEX idx_oauth_states_lookup;
