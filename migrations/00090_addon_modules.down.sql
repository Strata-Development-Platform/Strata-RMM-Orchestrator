DROP TRIGGER IF EXISTS addon_module_audit_no_delete ON addon_module_audit;
DROP TRIGGER IF EXISTS addon_module_audit_no_update ON addon_module_audit;
DROP FUNCTION IF EXISTS prevent_addon_module_audit_mutation();
DROP TABLE IF EXISTS addon_module_audit;
DROP TABLE IF EXISTS addon_modules;
