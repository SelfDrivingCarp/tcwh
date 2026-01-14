-- Schema v1

BEGIN;

CREATE TABLE tokens_issued (
    sub      TEXT     NOT NULL,
    iat      TEXT     NOT NULL,
    revoked  BOOLEAN  DEFAULT 0
);

CREATE INDEX revoked_tokens ON tokens_issued (revoked) WHERE (revoked=1);

CREATE TABLE oauth_tokens (
    sub             TEXT     NOT NULL PRIMARY KEY,
    twitch_id       TEXT     NOT NULL,
    twitch_login    TEXT     NOT NULL,
    last_validated  INTEGER  NOT NULL,
    refresh_token   TEXT     NOT NULL,
    access_token    TEXT     NOT NULL,
    expires_at      INTEGER  NOT NULL,
    scopes          TEXT     NOT NULL
);

CREATE TABLE webhooks (
    sub            TEXT     NOT NULL,
    label          TEXT     NOT NULL,
    template_type  TEXT     NOT NULL DEFAULT 'discord',
    template       TEXT,
    url            TEXT     NOT NULL UNIQUE,
    enabled        BOOLEAN  DEFAULT 1,
    UNIQUE(sub, label)
);

CREATE INDEX webhooks_by_sub ON webhooks (sub);

PRAGMA user_version=1;
PRAGMA optimize;

COMMIT;