package postgres

import (
	"strings"
	"testing"
)

func TestBootstrapAdminInputValidation(t *testing.T) {
	valid := BootstrapAdminInput{
		Email:      "owner@example.com",
		Password:   "correct horse battery staple",
		TenantName: "Example Platform",
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*BootstrapAdminInput)
	}{
		{name: "invalid email", mutate: func(in *BootstrapAdminInput) { in.Email = "not-an-email" }},
		{name: "short password", mutate: func(in *BootstrapAdminInput) { in.Password = "too-short" }},
		{name: "oversized password", mutate: func(in *BootstrapAdminInput) { in.Password = strings.Repeat("x", 73) }},
		{name: "empty tenant", mutate: func(in *BootstrapAdminInput) { in.TenantName = " " }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			tt.mutate(&input)
			if err := input.validate(); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}
