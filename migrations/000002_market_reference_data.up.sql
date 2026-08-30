CREATE TABLE IF NOT EXISTS currencies (
    code CHAR(3) PRIMARY KEY,
    symbol TEXT NOT NULL,
    minor_unit SMALLINT NOT NULL CHECK (minor_unit >= 0),
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS countries (
    code CHAR(2) PRIMARY KEY,
    default_currency_code CHAR(3) NOT NULL REFERENCES currencies(code),
    timezone TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS country_translations (
    country_code CHAR(2) NOT NULL REFERENCES countries(code) ON DELETE CASCADE,
    locale TEXT NOT NULL,
    name TEXT NOT NULL,
    PRIMARY KEY (country_code, locale)
);

CREATE TABLE IF NOT EXISTS markets (
    code CHAR(2) PRIMARY KEY,
    country_code CHAR(2) NOT NULL UNIQUE REFERENCES countries(code),
    currency_code CHAR(3) NOT NULL REFERENCES currencies(code),
    default_locale TEXT NOT NULL,
    timezone TEXT NOT NULL,
    status TEXT NOT NULL,
    configuration JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS market_locales (
    market_code CHAR(2) NOT NULL REFERENCES markets(code) ON DELETE CASCADE,
    locale TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (market_code, locale)
);

CREATE UNIQUE INDEX IF NOT EXISTS market_locales_one_default_idx
    ON market_locales (market_code)
    WHERE is_default;

INSERT INTO currencies (code, symbol, minor_unit, status) VALUES
    ('EGP', 'E£', 2, 'active'),
    ('SAR', 'SR', 2, 'active'),
    ('AED', 'AED', 2, 'active')
ON CONFLICT (code) DO NOTHING;

INSERT INTO countries (code, default_currency_code, timezone, status) VALUES
    ('EG', 'EGP', 'Africa/Cairo', 'active'),
    ('SA', 'SAR', 'Asia/Riyadh', 'active'),
    ('AE', 'AED', 'Asia/Dubai', 'active')
ON CONFLICT (code) DO NOTHING;

INSERT INTO country_translations (country_code, locale, name) VALUES
    ('EG', 'ar', 'مصر'),
    ('EG', 'en', 'Egypt'),
    ('SA', 'ar', 'السعودية'),
    ('SA', 'en', 'Saudi Arabia'),
    ('AE', 'ar', 'الإمارات العربية المتحدة'),
    ('AE', 'en', 'United Arab Emirates')
ON CONFLICT (country_code, locale) DO NOTHING;

INSERT INTO markets (code, country_code, currency_code, default_locale, timezone, status, configuration) VALUES
    ('EG', 'EG', 'EGP', 'ar', 'Africa/Cairo', 'active', '{"release_track":"launch"}'),
    ('SA', 'SA', 'SAR', 'ar', 'Asia/Riyadh', 'active', '{"release_track":"launch"}'),
    ('AE', 'AE', 'AED', 'ar', 'Asia/Dubai', 'active', '{"release_track":"launch"}')
ON CONFLICT (code) DO NOTHING;

INSERT INTO market_locales (market_code, locale, is_default, is_enabled, sort_order) VALUES
    ('EG', 'ar', true, true, 0),
    ('EG', 'en', false, true, 1),
    ('SA', 'ar', true, true, 0),
    ('SA', 'en', false, true, 1),
    ('AE', 'ar', true, true, 0),
    ('AE', 'en', false, true, 1)
ON CONFLICT (market_code, locale) DO NOTHING;
