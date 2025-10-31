-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS statuses
(
    name varchar PRIMARY KEY
);

INSERT INTO statuses(name)
values ('ACTIVE'),
       ('TIMEOUT'),
       ('DONE');

CREATE TABLE IF NOT EXISTS op_type
(
    name varchar PRIMARY KEY,
    code bigint not null unique
);

INSERT INTO op_type(name, code)
values ('CONTRIBUTE', 0x0f325335),
       ('TRANSFER', 0x6ffa34c0),
       ('REFUND', 0xc0d15cf0);

CREATE TABLE IF NOT EXISTS bills
(
    id              uuid PRIMARY KEY,
    goal            bigint    not null,
    collected       bigint    not null default 0,
    creator_address varchar   not null,
    dest_address    varchar   not null,
    created_at      timestamp not null default now(),
    status          varchar REFERENCES statuses (name)
);

CREATE TABLE IF NOT EXISTS transactions
(
    id             uuid PRIMARY KEY,
    bill_id        uuid REFERENCES bills (id),
    amount         bigint    not null,
    sender_address varchar   not null,
    created_at     timestamp not null default now(),
    op_type        varchar REFERENCES op_type (name)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE transactions CASCADE;
DROP TABLE bills CASCADE;
DROP TABLE op_type CASCADE;
DROP TABLE statuses CASCADE;
-- +goose StatementEnd
