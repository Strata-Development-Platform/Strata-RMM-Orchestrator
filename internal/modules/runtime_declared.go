package modules

import "context"

// InvokeDeclared executes a manifest-declared route while deriving its required
// permission from the installed manifest. Callers supply method, path, body, and
// trusted host scope only; they cannot choose or downgrade the permission that
// Supervisor enforces for the route.
func (s *Supervisor) InvokeDeclared(ctx context.Context, id, method, path string, body []byte, scope ResourceScope) (InvocationResult, error) {
	module, err := s.enabledModule(id)
	if err != nil {
		return InvocationResult{}, err
	}
	route, err := declaredRoute(module.Manifest, method, path)
	if err != nil {
		return InvocationResult{}, err
	}
	return s.Invoke(ctx, id, Invocation{
		Method:     method,
		Path:       path,
		Permission: route.Permission,
		Body:       body,
		Scope:      scope,
	})
}
