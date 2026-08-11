package modules

import (
	"database/sql"
	"testing"
)

func TestVersionAuditActionsPreserveLifecycleState(t *testing.T) {
	for _, action := range []string{"upgrade", "rollback"} {
		for _, state := range []State{StateInstalled, StateDisabled} {
			previous := sql.NullString{String: string(state), Valid: true}
			if err := validateAuditTransition(previous, state, action); err != nil {
				t.Fatalf("%s %s -> %s rejected: %v", action, state, state, err)
			}
			if err := validateAuditTransition(previous, StateEnabled, action); err == nil {
				t.Fatalf("%s unexpectedly allowed lifecycle change %s -> enabled", action, state)
			}
		}
	}
}

func TestVersionAuditActionsRequireExistingModule(t *testing.T) {
	for _, action := range []string{"upgrade", "rollback"} {
		if err := validateAuditTransition(sql.NullString{}, StateInstalled, action); err == nil {
			t.Fatalf("%s unexpectedly accepted for missing module", action)
		}
		if !validAuditAction(action) {
			t.Fatalf("%s is not recognized as an audit action", action)
		}
	}
}
