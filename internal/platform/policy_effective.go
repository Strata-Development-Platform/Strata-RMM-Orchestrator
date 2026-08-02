package platform

func computeEffectiveConfig(layers []policyLayer) map[string]interface{} {
	return mergePolicyLayers(layers)
}

func computeEffectiveMaintenance(layers []policyLayer) map[string]interface{} {
	effective := map[string]interface{}{}
	for _, layer := range layers {
		if layer.MaintenanceStart != nil {
			effective["maintenance_start"] = *layer.MaintenanceStart
		}
		if layer.MaintenanceEnd != nil {
			effective["maintenance_end"] = *layer.MaintenanceEnd
		}
		if layer.MaintenanceDays != nil && len(*layer.MaintenanceDays) > 0 {
			effective["maintenance_days"] = *layer.MaintenanceDays
		}
		if layer.MaintenanceTimezone != "" {
			effective["maintenance_timezone"] = layer.MaintenanceTimezone
		}
	}
	return effective
}
