-- Reverse migration for 000007_theme_engine_schema.
-- Drops the theme domain tables in dependency order. Theme data is presentation
-- configuration only and carries no commerce source-of-truth, so dropping is safe.

DROP TABLE IF EXISTS theme_assets;
DROP TABLE IF EXISTS theme_configurations;
DROP TABLE IF EXISTS theme_installations;
DROP TABLE IF EXISTS theme_versions;
DROP TABLE IF EXISTS themes;
