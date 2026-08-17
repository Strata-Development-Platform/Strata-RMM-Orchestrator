# Representative Host Internal-Alpha Acceptance

This runbook closes the environment-dependent Windows and Linux gates in `PATCH_FLEET_ALPHA_ACCEPTANCE.md`. Repository tests do not satisfy these checks. Evidence must come from enrolled endpoints running artifacts built from the final candidate SHA.

## Rules

1. Freeze one candidate SHA before host validation. Record the full SHA in every evidence capture.
2. Install/enroll the agent built from that SHA on one representative Windows endpoint and one representative Linux endpoint.
3. Initiate patch and software actions through the Strata RMM product path. Do not replace the product action with local `apt`, `dnf`, Windows Update, MSI, or shell commands.
4. Use the platform-specific evidence collector immediately before and after each product action.
5. Preserve platform-side deployment/job identifiers, target status, timestamps, and relevant agent/orchestrator logs with the host evidence.
6. Never claim success merely because the host changed. The corresponding Strata job/deployment must converge to the expected terminal state.
7. Do not reboot automatically for acceptance. If Windows reports reboot required, capture that state and prove Strata surfaced it without initiating a reboot.

## Linux endpoint

Before any action:

```bash
sudo scripts/alpha-host-evidence-linux.sh \
  --candidate-sha <FULL_SHA> --phase before \
  --package <TEST_PACKAGE>
```

Then, through Strata RMM:

1. Run a patch scan and verify the device inventory is updated.
2. Select one safe missing package/update and deploy it through the patch workflow.
3. Verify the durable job target and patch deployment converge successfully.
4. Deploy a representative software package through Software Deployment. A script package is acceptable; DEB/RPM is preferred where the host supports it.
5. If uninstall is supported for the chosen package, run uninstall through Strata and verify the terminal result.

Capture after state:

```bash
sudo scripts/alpha-host-evidence-linux.sh \
  --candidate-sha <FULL_SHA> --phase after \
  --package <TEST_PACKAGE>
```

Linux acceptance requires retained evidence showing: agent active; authoritative patch scan; real host package state changed as requested; platform target/deployment terminal success; no cross-tenant/device execution; and software deployment result consistent with host state.

## Windows endpoint

Open an elevated PowerShell prompt and capture before state:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\alpha-host-evidence-windows.ps1 `
  -CandidateSha <FULL_SHA> -Phase before -PackageName '<TEST_PACKAGE>'
```

Then, through Strata RMM:

1. Run a Windows Update scan and verify missing updates are surfaced for the enrolled device.
2. Select one safe applicable update and deploy it through the patch workflow.
3. Verify the durable target/deployment reaches the correct terminal state.
4. If the Windows Update API reports reboot required, verify Strata persists/surfaces `reboot_required` and does not reboot the endpoint automatically.
5. Deploy one supported software package through Software Deployment and verify host installation state and platform convergence.
6. Where uninstall is supported, uninstall through Strata and verify both host and platform state.

Capture after state:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\alpha-host-evidence-windows.ps1 `
  -CandidateSha <FULL_SHA> -Phase after -PackageName '<TEST_PACKAGE>'
```

Windows acceptance requires retained evidence showing: agent service state; Windows Update before/after inventory; actual installed product/update state; platform terminal result; reboot-required state when applicable; and no automatic reboot initiated by Strata.

## Evidence package

For each host retain a directory containing the collector output plus a short `platform.txt` with:

```text
candidate_sha=<full SHA>
hostname=<endpoint hostname>
device_id=<Strata device id>
agent_id=<Strata agent id>
tenant/msp/client/site=<scope>
patch_deployment_id=<id>
patch_job_id=<id>
patch_target_id=<id>
software_deployment_id=<id>
software_job_id=<id>
software_target_id=<id>
started_at=<UTC timestamp>
completed_at=<UTC timestamp>
result=<pass/fail and reason>
```

Attach or otherwise retain the evidence with the PR/issue before marking the representative-host rows complete. If the candidate SHA changes afterward, repeat representative-host validation against the new candidate.

## Pass/fail gate

PR #148 may leave draft only when the final candidate SHA has terminal-green required repository workflows and both representative host evidence sets are retained. A missing update/package on a test host is not a pass; select a host/workload that can exercise the requested production path. Any scope mismatch, lost result, premature success, unrequested reboot, or host/platform state disagreement is a blocker.
