-- +goose Up
INSERT INTO
    schools (name)
VALUES
    ('Brown University') ON CONFLICT (name) DO NOTHING;

-- +goose Down
DELETE FROM
    schools
WHERE
    name IN ('Brown University');