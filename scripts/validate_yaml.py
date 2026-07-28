#!/usr/bin/env python3
"""Validate all YAML files in the repository."""
import os, sys, yaml

errors = []
for root, dirs, files in os.walk('.'):
    if 'node_modules' in root or '.git' in root or '/templates' in root:
        continue
    for f in files:
        if not (f.endswith('.yml') or f.endswith('.yaml')):
            continue
        path = os.path.join(root, f)
        try:
            yaml.safe_load(open(path))
        except Exception as e:
            errors.append(f'{path}: {e}')

if errors:
    for e in errors:
        print(e, file=sys.stderr)
    sys.exit(1)
print("All YAML files valid")
