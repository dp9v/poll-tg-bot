-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS staff (
    id             BIGINT      PRIMARY KEY,
    name           TEXT        NOT NULL,
    specialization TEXT        NOT NULL DEFAULT '',
    avatar         TEXT        NOT NULL DEFAULT '',
    rating         NUMERIC     NOT NULL DEFAULT 0,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS categories (
    id         BIGINT      PRIMARY KEY,
    title      TEXT        NOT NULL,
    parent_id  BIGINT      NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS services (
    id          BIGINT      PRIMARY KEY,
    title       TEXT        NOT NULL,
    category_id BIGINT      NOT NULL DEFAULT 0,
    price_min   INTEGER     NOT NULL DEFAULT 0,
    price_max   INTEGER     NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- The activities table now references staff/services by id only;
-- denormalized name/title columns are gone.
-- +goose StatementBegin
ALTER TABLE activities DROP COLUMN IF EXISTS staff_name;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE activities DROP COLUMN IF EXISTS service_title;
-- +goose StatementEnd

-- Helpful indexes for joins.
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS activities_service_id_idx ON activities (service_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS activities_staff_id_idx ON activities (staff_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS services_category_id_idx ON services (category_id);
-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS services_category_id_idx;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS activities_staff_id_idx;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS activities_service_id_idx;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE activities ADD COLUMN service_title TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE activities ADD COLUMN staff_name TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS services;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS categories;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS staff;
-- +goose StatementEnd

