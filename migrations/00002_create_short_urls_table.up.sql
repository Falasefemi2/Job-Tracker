CREATE TABLE short_urls (
    code TEXT PRIMARY KEY,
    long_url TEXT NOT NULL,
    clicks BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT short_urls_long_url_unique UNIQUE (long_url)
);
