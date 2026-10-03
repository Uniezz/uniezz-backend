-- +goose Up

CREATE TABLE otp_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    university_id VARCHAR(32) NOT NULL,
    email VARCHAR(255) NOT NULL,
    code_hash BYTEA NOT NULL,
    attempts_count INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    ip INET NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_otp_requests_lookup 
ON otp_requests (university_id, LOWER(email), created_at DESC)
WHERE consumed_at IS NULL AND cancelled_at IS NULL;

CREATE INDEX idx_otp_requests_email_rate_limit 
ON otp_requests (university_id, LOWER(email), created_at);

CREATE INDEX idx_otp_requests_ip_rate_limit 
ON otp_requests (ip, created_at);


-- +goose Down
DROP TABLE otp_requests;
