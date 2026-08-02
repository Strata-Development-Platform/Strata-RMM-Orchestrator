ALTER TABLE policies ADD COLUMN IF NOT EXISTS maintenance_start TIME;
ALTER TABLE policies ADD COLUMN IF NOT EXISTS maintenance_end TIME;
ALTER TABLE policies ADD COLUMN IF NOT EXISTS maintenance_days JSONB DEFAULT '["monday","tuesday","wednesday","thursday","friday"]';
ALTER TABLE policies ADD COLUMN IF NOT EXISTS maintenance_timezone TEXT DEFAULT 'UTC';

ALTER TABLE policy_revisions ADD COLUMN IF NOT EXISTS maintenance_start TIME;
ALTER TABLE policy_revisions ADD COLUMN IF NOT EXISTS maintenance_end TIME;
ALTER TABLE policy_revisions ADD COLUMN IF NOT EXISTS maintenance_days JSONB DEFAULT '["monday","tuesday","wednesday","thursday","friday"]';
ALTER TABLE policy_revisions ADD COLUMN IF NOT EXISTS maintenance_timezone TEXT DEFAULT 'UTC';

CREATE OR REPLACE VIEW policy_effective_config AS
WITH RECURSIVE policy_hierarchy AS (
    SELECT 
        p.id,
        p.msp_id,
        p.client_id,
        p.site_id,
        p.device_id,
        p.category,
        p.scope_level,
        p.version,
        p.config,
        p.maintenance_start,
        p.maintenance_end,
        p.maintenance_days,
        p.maintenance_timezone,
        p.published_version,
        p.status,
        1 AS depth
    FROM policies p
    WHERE p.status = 'active' AND p.published_version IS NOT NULL
    UNION ALL
    SELECT 
        p.id,
        ph.msp_id,
        COALESCE(p.client_id, ph.client_id),
        COALESCE(p.site_id, ph.site_id),
        COALESCE(p.device_id, ph.device_id),
        p.category,
        p.scope_level,
        p.version,
        p.config,
        p.maintenance_start,
        p.maintenance_end,
        p.maintenance_days,
        p.maintenance_timezone,
        p.published_version,
        p.status,
        ph.depth + 1
    FROM policies p
    INNER JOIN policy_hierarchy ph ON (
        (ph.scope_level = 'device' AND p.scope_level = 'site' AND p.site_id = ph.site_id)
        OR (ph.scope_level IN ('site', 'device') AND p.scope_level = 'client' AND p.client_id = ph.client_id)
        OR (ph.scope_level IN ('client', 'site', 'device') AND p.scope_level = 'msp')
    )
    WHERE p.status = 'active' AND p.published_version IS NOT NULL AND p.category = ph.category
),
layered_config AS (
    SELECT 
        id,
        msp_id,
        client_id,
        site_id,
        device_id,
        category,
        scope_level,
        version,
        config,
        maintenance_start,
        maintenance_end,
        maintenance_days,
        maintenance_timezone,
        depth,
        ROW_NUMBER() OVER (PARTITION BY id ORDER BY depth DESC) AS rn
    FROM policy_hierarchy
),
effective_layers AS (
    SELECT 
        id,
        msp_id,
        client_id,
        site_id,
        device_id,
        category,
        scope_level,
        version,
        config,
        maintenance_start,
        maintenance_end,
        maintenance_days,
        maintenance_timezone,
        depth
    FROM layered_config
    WHERE rn = 1
),
all_layers AS (
    SELECT 
        el.id,
        el.msp_id,
        el.client_id,
        el.site_id,
        el.device_id,
        el.category,
        el.scope_level AS effective_scope,
        el.version AS effective_version,
        el.config AS effective_config,
        el.maintenance_start AS effective_maintenance_start,
        el.maintenance_end AS effective_maintenance_end,
        el.maintenance_days AS effective_maintenance_days,
        el.maintenance_timezone AS effective_maintenance_timezone,
        p.id AS layer_id,
        p.scope_level AS layer_scope,
        p.version AS layer_version,
        p.config AS layer_config,
        p.maintenance_start AS layer_maintenance_start,
        p.maintenance_end AS layer_maintenance_end,
        p.maintenance_days AS layer_maintenance_days,
        p.maintenance_timezone AS layer_maintenance_timezone
    FROM effective_layers el
    CROSS JOIN policy_hierarchy ph
    INNER JOIN policies p ON p.id = ph.id
    WHERE ph.id = el.id
    ORDER BY ph.depth DESC
),
agg_layers AS (
    SELECT 
        layer_id,
        layer_scope,
        layer_version,
        layer_config,
        layer_maintenance_start,
        layer_maintenance_end,
        layer_maintenance_days,
        layer_maintenance_timezone
    FROM all_layers
    WHERE all_layers.id = all_layers.layer_id
)
SELECT 
    el.id AS policy_id,
    el.msp_id,
    el.client_id,
    el.site_id,
    el.device_id,
    el.category,
    el.effective_scope,
    el.effective_version,
    el.effective_config,
    el.effective_maintenance_start,
    el.effective_maintenance_end,
    el.effective_maintenance_days,
    el.effective_maintenance_timezone,
    COALESCE(json_agg(DISTINCT json_build_object(
        'id', al.layer_id,
        'scope_level', al.layer_scope,
        'version', al.layer_version,
        'config', al.layer_config,
        'maintenance_start', al.layer_maintenance_start,
        'maintenance_end', al.layer_maintenance_end,
        'maintenance_days', al.layer_maintenance_days,
        'maintenance_timezone', al.layer_maintenance_timezone
    )) FILTER (WHERE al.layer_id IS NOT NULL), '[]') AS layers,
    json_build_object(
        'id', el.id,
        'msp_id', el.msp_id::text,
        'client_id', COALESCE(el.client_id::text, ''),
        'site_id', COALESCE(el.site_id::text, ''),
        'device_id', COALESCE(el.device_id::text, ''),
        'category', el.category,
        'scope_level', el.effective_scope,
        'version', el.effective_version,
        'config', el.effective_config,
        'maintenance_start', el.effective_maintenance_start,
        'maintenance_end', el.effective_maintenance_end,
        'maintenance_days', el.effective_maintenance_days,
        'maintenance_timezone', el.effective_maintenance_timezone
    ) AS effective_with_sources
FROM effective_layers el
LEFT JOIN all_layers al ON al.id = el.id AND al.layer_id = el.id
GROUP BY 
    el.id, el.msp_id, el.client_id, el.site_id, el.device_id,
    el.category, el.effective_scope, el.effective_version, el.effective_config,
    el.effective_maintenance_start, el.effective_maintenance_end,
    el.effective_maintenance_days, el.effective_maintenance_timezone;
