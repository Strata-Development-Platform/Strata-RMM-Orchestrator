# Agent Engineering Ledger — broker device read slice

Base: `72baf3af4f234cffdb3800b9f30c56cbe21f1ff6` (master after PR #134)

This slice implements the first concrete read-only WASI broker capability: `devices.get`.

Key rules reinforced:

- Capability permission is fixed by the host (`devices.read`), never guest-selected.
- Guest input contains only a bounded device identifier.
- The trusted organizational scope comes from host invocation context, never guest JSON.
- The handler resolves device identity and ownership through a trusted resolver and requires the resolved scope to exactly match the trusted invocation target.
- Unknown input fields, multiple JSON values, missing IDs, and oversized IDs fail closed.
- Only a minimal device projection is returned; scope and backend error text are not exposed through the WASI ABI.
- Sibling-site substitution and resolver identity mismatch are explicit negative tests.

Do not broaden this operation by accepting MSP/client/site/tenant IDs from guest input. Do not add raw database, filesystem, NATS, secret, or network handles to satisfy future capability requirements.
