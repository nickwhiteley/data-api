-- +goose Up
-- +goose StatementBegin

ALTER TYPE wd.user_role ADD VALUE IF NOT EXISTS 'data_engineer';

-- +goose StatementEnd

-- +goose Down
-- No reverse for enum value removal in PostgreSQL without recreating the type.
