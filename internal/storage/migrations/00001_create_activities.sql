-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS activities (
    id            BIGINT  PRIMARY KEY,
    date          TEXT    NOT NULL,
    capacity      INTEGER NOT NULL,
    records_count INTEGER NOT NULL,
    staff_id      BIGINT  NOT NULL,
    staff_name    TEXT    NOT NULL,
    service_id    BIGINT  NOT NULL,
    service_title TEXT    NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS activities;
-- +goose StatementEnd

