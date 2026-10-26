-- +goose Up
DROP INDEX idx_otp_requests_lookup;
ALTER TABLE otp_requests DROP COLUMN ip;
ALTER TABLE otp_requests DROP COLUMN cancelled_at;
ALTER INDEX idx_otp_requests_email_rate_limit RENAME TO idx_otp_requests_lookup;

-- +goose Down
ALTER INDEX idx_otp_requests_lookup RENAME TO idx_otp_requests_email_rate_limit;
ALTER TABLE otp_requests ADD COLUMN cancelled_at TIMESTAMPTZ;
ALTER TABLE otp_requests ADD COLUMN ip INET;

CREATE INDEX idx_otp_requests_ip_rate_limit
ON otp_requests (ip, created_at);

CREATE INDEX idx_otp_requests_lookup
ON otp_requests (university_id, LOWER(email), created_at DESC)
WHERE consumed_at IS NULL AND cancelled_at IS NULL;
