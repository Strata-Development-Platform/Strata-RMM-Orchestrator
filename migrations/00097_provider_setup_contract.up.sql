ALTER TABLE platforms
    ADD COLUMN IF NOT EXISTS provider_logo_light_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_logo_dark_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_favicon_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_brand_light_color TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_brand_dark_color TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_terms_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_privacy_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_support_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS public_saas_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS public_saas_headline TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS public_saas_description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS setup_contract_version INTEGER NOT NULL DEFAULT 1;

ALTER TABLE platforms
    ADD CONSTRAINT platforms_provider_brand_light_color_format
        CHECK (provider_brand_light_color = '' OR provider_brand_light_color ~ '^#[0-9A-Fa-f]{6}$'),
    ADD CONSTRAINT platforms_provider_brand_dark_color_format
        CHECK (provider_brand_dark_color = '' OR provider_brand_dark_color ~ '^#[0-9A-Fa-f]{6}$'),
    ADD CONSTRAINT platforms_setup_contract_version_positive
        CHECK (setup_contract_version >= 1);

COMMENT ON COLUMN platforms.setup_contract_version IS
    'Authoritative first-login provider setup contract version. Current pre-beta contract is version 2.';
