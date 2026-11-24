-- +goose Up
ALTER VIEW anonymous_profiles RENAME TO discovery_profiles;

-- +goose Down
ALTER VIEW discovery_profiles RENAME TO anonymous_profiles;
