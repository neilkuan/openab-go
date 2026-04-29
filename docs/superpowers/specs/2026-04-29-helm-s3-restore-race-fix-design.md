# Helm Chart — Fix S3 Backup Restore Race on Pod Startup

**Date:** 2026-04-29
**Branch:** `release/v0.22.2` (working branch — actual implementation branch chosen by writing-plans)
**Status:** Spec — pending implementation plan
**Supersedes (partially):** `2026-04-27-helm-s3-backup-design.md` — Architecture section only. Goals, non-goals, values schema, auth modes are unchanged.

## Background

The S3 backup feature shipped in 2026-04-27 uses a single Kubernetes 1.28+
native sidecar (`initContainers` with `restartPolicy: Always`) to handle
both directions:

- **On startup**: `rclone copy s3://… /workdir`, then `while true; do sleep 3600; done`
- **On termination**: `lifecycle.preStop` runs `rclone sync /workdir s3://…`

Field reports show that S3 receives backups (preStop side works), but pods
restart with an empty `/home/agent` (restore side fails). Confirmed by:

```
$ aws s3 ls s3://aws-tperd-splashtop-premium-tool-ap-northeast-1 --recursive
2026-04-29 07:40:31         39 quill/kiro/.bash_history
2026-04-29 07:40:32         19 quill/kiro/.kiro/.cli_bash_history
2026-04-29 09:19:04      36864 quill/kiro/.local/share/kiro-cli/data.sqlite3
2026-04-29 07:40:32     466247 quill/kiro/.semantic_search/models/all-MiniLM-L6-v2/tokenizer.json
```

S3 has session data, but the Kiro agent reports an empty session list after
pod restart.

## Root cause

Two independent defects, both of which must be fixed for restore to work:

### Defect 1 — startup race (no synchronous gate)

Native sidecar startup ordering only guarantees that the sidecar **process
has started** before the main container is admitted — it does **not** wait
for the sidecar's entrypoint command to complete. The current sidecar
script is:

```sh
rclone copy "s3:${BUCKET}/${PREFIX}" /workdir --create-empty-src-dirs $EXTRA_ARGS || true
while true; do sleep 3600; done
```

The kubelet sees the `sh` process running, marks the sidecar `Started`, and
admits the main container. Meanwhile, `rclone copy` is still streaming
files. The Kiro agent boots, scans `/home/agent`, finds it empty (or
partially populated), and reports no resumable sessions. Files arriving
later are correct on disk but already missed by the agent's startup scan.

### Defect 2 — file ownership mismatch

The `rclone/rclone:1.66` image runs as **root (UID 0)** by default. Files
written into the shared `emptyDir` by `rclone copy` end up owned by
`root:root`. The quill main container runs as **UID 1000** in every
Dockerfile variant (`agent` user in `Dockerfile`, `node` user in the three
`Dockerfile.{claude,codex,copilot}` — both UID 1000). With files owned by
root and mode `0644`, the agent process (UID 1000):

- Can read most files (world-readable),
- **Cannot** write to existing files (no write bit for `other`),
- **Cannot** write into restored sub-directories (typically `0755 root`),
- Fails to acquire SQLite locks on `data.sqlite3` (write attempt → EACCES).

Result: even after a successful `rclone copy`, the agent treats the
working directory as broken — file open errors during session-list scan
look identical to "directory is empty" from the user's perspective.
Defect 2 alone explains the observed symptom even when defect 1 is masked
by lucky timing (small bucket, fast network).

### Why these two defects compound

The current single-sidecar layout has neither a synchronous gate (defect
1) nor explicit ownership control (defect 2). The fix below addresses
both: the new init-container layout closes the race, and explicit
`securityContext` on the rclone containers + pod-level `fsGroup` ensures
the restored files are owned by UID:GID 1000:1000 (matching the quill
main container's runtime user), so the agent has full read/write access.

## Goals

- Pod start → `/home/agent` is fully hydrated from S3 **before** the quill
  main container's process begins, every time.
- Restored files are owned by **UID:GID 1000:1000** so the quill agent
  process can read and write them without permission errors.
- Pod terminates gracefully → unchanged from the 2026-04-27 design (preStop
  sync continues to work as today).
- Minimal `values.yaml` schema change: a new optional `backup.ownership`
  block (with sensible 1000:1000 defaults) so non-1000 images can override.
  Existing values files keep working with no changes.
- Auth modes, Dockerfile variants, and the `replicas: 1` constraint
  unchanged.
- Disabled-by-default behaviour preserved (`backup.enabled=false` keeps the
  current single-emptyDir layout).

## Non-goals

- Periodic mid-flight sync — explicitly deferred to a follow-up; abrupt
  termination (OOMKill, node failure) still loses data back to last preStop.
- Aligning the sidecar's mountPath to per-instance `agent.workingDir`
  (`/home/agent` vs `/home/node`). Sidecar template stays at `/workdir`
  because the same template serves multiple instances; the underlying
  emptyDir volume is shared so file contents are identical across paths.
- Increasing `terminationGracePeriodSeconds`. Current 60s stays; revisit
  only if backup-side timeouts surface separately.
- Migrating to PVC, csi-s3, or other durable-volume approaches.

## Architecture

Replace the single sidecar with two cooperating containers sharing the
existing `workdir` emptyDir volume:

```
Pod created
  │
  ├─ initContainers (run in declaration order; each must exit before next)
  │  │
  │  ├─ s3-restore                     ← plain init container
  │  │   rclone copy s3://…/{instance} /workdir
  │  │   exits 0 → kubelet advances to next initContainer
  │  │
  │  └─ s3-backup (sidecar)            ← restartPolicy: Always
  │      exec sleep infinity
  │      preStop: rclone sync /workdir s3://…/{instance}
  │      kubelet sees process started → admits main container
  │
  └─ containers
     └─ quill                          ← /home/agent already populated
        agent boots, scans dir, sees full session history
```

Key shift: **restore moves out of the sidecar into a regular init
container**. Regular init containers must exit before the next container
in the spec is admitted, which gives us the synchronous gate the current
design lacks. The sidecar shrinks to "idle process that holds a preStop
hook" — its only job is to live long enough to receive SIGTERM after main
exits, so the preStop fires.

### Termination flow (unchanged from 2026-04-27)

1. Pod receives termination signal.
2. Main container `quill` receives SIGTERM, drains, exits.
3. Sidecar `s3-backup` receives SIGTERM (kubelet ordering: sidecars after
   main containers).
4. `lifecycle.preStop` hook executes `rclone sync` before the sidecar
   process is sent SIGTERM.
5. Sidecar exits, pod terminated.

This path was working in the 2026-04-27 implementation and we keep it
verbatim.

## Components

### `deploy/helm/quill/values.yaml` — add `backup.ownership` block

```yaml
backup:
  # ... existing fields unchanged ...
  ownership:
    # UID/GID rclone containers run as. Files restored from S3 are owned
    # by these IDs, so they must match the UID/GID inside the quill main
    # container. All four built-in agent images run as 1000:1000.
    # Override only if you build a custom image with a different runtime user.
    runAsUser: 1000
    runAsGroup: 1000
    # Pod-level fsGroup applied to the shared emptyDir volume. Kubernetes
    # recursively chowns volume contents to GID at mount time and sets
    # the SGID bit so subsequent files inherit the GID. Set to the same
    # value as runAsGroup unless you have a specific reason to differ.
    fsGroup: 1000
```

### `deploy/helm/quill/templates/deployment.yaml`

Replace the current single-`s3-sync` initContainer block with two entries
(both gated by `{{ if $b.enabled }}`) and add a pod-level `securityContext`
so `fsGroup` chown propagates to the shared `emptyDir`.

**Pod-level securityContext** (only when `backup.enabled` — keeps the
disabled path unchanged):

```yaml
spec:
  template:
    spec:
      {{- if $b.enabled }}
      securityContext:
        fsGroup: {{ $b.ownership.fsGroup }}
      {{- end }}
      ...
```

**Container 1: `s3-restore`** (no `restartPolicy` field — plain init)

```yaml
- name: s3-restore
  image: "{{ $b.rclone.image }}:{{ $b.rclone.tag }}"
  imagePullPolicy: {{ $b.rclone.pullPolicy }}
  securityContext:
    runAsUser: {{ $b.ownership.runAsUser }}
    runAsGroup: {{ $b.ownership.runAsGroup }}
    runAsNonRoot: true
  command:
    - /bin/sh
    - -c
    - |
      set -eu
      rclone copy "s3:${BUCKET}/${PREFIX}" /workdir --create-empty-src-dirs $EXTRA_ARGS
  env:
    - name: HOME
      value: /tmp                      # rclone needs a writable HOME for cache
    {{- include "quill.s3.envVars" (dict "ctx" $ "name" $name "b" $b) | nindent 4 }}
  volumeMounts:
    - name: workdir
      mountPath: /workdir
  resources:
    {{- toYaml $b.resources | nindent 4 }}
```

Note the removal of `|| true`. If S3 access is misconfigured (wrong bucket,
expired creds, missing IRSA), we want the pod to CrashLoop loudly rather
than silently start with an empty directory and proceed to overwrite the
S3 backup with nothing on the next preStop. First-time deploy with an
empty prefix is a non-issue: `rclone copy` against a non-existent prefix
returns 0 (no files to copy is not an error).

**Container 2: `s3-backup`** (sidecar — `restartPolicy: Always`)

```yaml
- name: s3-backup
  image: "{{ $b.rclone.image }}:{{ $b.rclone.tag }}"
  imagePullPolicy: {{ $b.rclone.pullPolicy }}
  restartPolicy: Always
  securityContext:
    runAsUser: {{ $b.ownership.runAsUser }}
    runAsGroup: {{ $b.ownership.runAsGroup }}
    runAsNonRoot: true
  command:
    - /bin/sh
    - -c
    - exec sleep infinity
  lifecycle:
    preStop:
      exec:
        command:
          - /bin/sh
          - -c
          - |
            set -eu
            rclone sync /workdir "s3:${BUCKET}/${PREFIX}" --create-empty-src-dirs $EXTRA_ARGS
  env:
    - name: HOME
      value: /tmp
    {{- include "quill.s3.envVars" (dict "ctx" $ "name" $name "b" $b) | nindent 4 }}
  volumeMounts:
    - name: workdir
      mountPath: /workdir
  resources:
    {{- toYaml $b.resources | nindent 4 }}
```

### Ownership model — what each piece guarantees

| Mechanism | Guarantees |
|---|---|
| `runAsUser: 1000`, `runAsGroup: 1000` on rclone containers | Files written by `rclone copy` and read by `rclone sync` are owned by UID 1000, GID 1000 — directly matching the quill main container's runtime user. |
| `runAsNonRoot: true` | Defence-in-depth: pod fails admission if the rclone image somehow ships with `USER 0`, preventing accidental root-owned writes. |
| Pod-level `fsGroup: 1000` | Kubelet recursively chowns the `emptyDir` contents to GID 1000 at mount time **and** sets the SGID bit so future files inherit the GID. Covers the edge case where a custom image runs rclone as a different UID but shares the GID. |
| `HOME=/tmp` env on rclone containers | rclone tries to write `~/.cache/rclone/` for transfer state; UID 1000 has no entry in the rclone Alpine image's `/etc/passwd`, so its default HOME resolves to `/`, which is read-only. Pointing HOME at the writable `/tmp` tmpfs avoids the `mkdir: permission denied` warning. |

`exec sleep infinity` replaces `while true; do sleep 3600; done`:

- `exec` makes `sleep` PID 1, so SIGTERM is delivered cleanly.
- `sleep infinity` (BusyBox supports this) ends immediately on SIGTERM
  without needing to wait out a 3600s tick.
- preStop runs synchronously before the SIGTERM is sent, so this only
  matters if preStop hangs and grace period expires — but it does mean
  the SIGKILL fallback is responsive.

### New helper: `deploy/helm/quill/templates/_helpers.tpl`

Both rclone containers share identical env vars. Move the duplicated env
list into a named template:

```gotmpl
{{- define "quill.s3.envVars" -}}
- name: BUCKET
  value: {{ .b.s3.bucket | quote }}
- name: PREFIX
  value: "{{ .b.s3.prefix }}/{{ .name }}"
- name: RCLONE_CONFIG_S3_TYPE
  value: s3
- name: RCLONE_CONFIG_S3_PROVIDER
  value: AWS
- name: RCLONE_CONFIG_S3_REGION
  value: {{ .b.s3.region | quote }}
- name: RCLONE_CONFIG_S3_ENV_AUTH
  value: "true"
{{- if .b.s3.endpoint }}
- name: RCLONE_CONFIG_S3_ENDPOINT
  value: {{ .b.s3.endpoint | quote }}
{{- end }}
- name: EXTRA_ARGS
  value: {{ join " " .b.rclone.extraArgs | quote }}
{{- if eq .b.auth.mode "secret" }}
- name: AWS_ACCESS_KEY_ID
  valueFrom:
    secretKeyRef:
      name: {{ .b.auth.secret.existingSecret | default (printf "%s-%s-s3-creds" (include "quill.fullname" .ctx) .name) }}
      key: AWS_ACCESS_KEY_ID
- name: AWS_SECRET_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .b.auth.secret.existingSecret | default (printf "%s-%s-s3-creds" (include "quill.fullname" .ctx) .name) }}
      key: AWS_SECRET_ACCESS_KEY
{{- end }}
{{- end -}}
```

Caller pattern: `{{- include "quill.s3.envVars" (dict "ctx" $ "name" $name "b" $b) | nindent 12 }}` (indent depth depends on caller's nesting level inside the YAML).

### `deploy/helm/quill/Chart.yaml`

Bump chart `version` (semver patch — bug fix to existing feature, no
schema change). `appVersion` unchanged.

### Unchanged

- `values.yaml` — `backup` block schema is identical.
- `templates/serviceaccount.yaml` — IRSA / podIdentity SA logic unchanged.
- `templates/secret-s3.yaml` — secret-mode credential generation unchanged.
- All other templates.

## Tests

### `tests/template-tests.sh` — new assertions

Existing tests (`values-irsa.yaml`, `values-pod-identity.yaml`,
`values-secret-existing.yaml`, `values-secret-inline.yaml`) all enable
backup. Extend the harness to assert:

1. **Two initContainers exist when backup enabled**
   `helm template . -f tests/values-irsa.yaml` produces a Deployment with
   exactly two `initContainers` entries: `s3-restore` then `s3-backup`.
2. **Order matters**: `s3-restore` appears first, `s3-backup` second.
3. **`s3-restore` is a plain init**: no `restartPolicy` field.
4. **`s3-backup` is a native sidecar**: `restartPolicy: Always`.
5. **`s3-restore` carries no `lifecycle.preStop`**.
6. **`s3-backup` carries `lifecycle.preStop`** with an `rclone sync …`
   command.
7. **`s3-restore` command contains `rclone copy`** but **not `sleep`**.
8. **`s3-backup` command contains `sleep infinity`** but **not `rclone copy`**.
9. **Both rclone containers have `runAsUser: 1000`, `runAsGroup: 1000`,
   `runAsNonRoot: true`** in their `securityContext`.
10. **Pod template has `securityContext.fsGroup: 1000`** when backup is
    enabled.
11. **Both rclone containers have `HOME=/tmp` env var**.
12. **`backup.enabled: false` produces zero initContainers and no
    pod-level `fsGroup`** (regression guard for the disabled path).
13. **Custom ownership values propagate**: a new `tests/values-custom-uid.yaml`
    with `backup.ownership.runAsUser: 2000` (and matching gid/fsGroup)
    must render those values into the security contexts.
14. **All three auth modes still render** (existing assertions retained).

Implementation: extend the bash assertions in `tests/template-tests.sh`
using `yq` selectors against the rendered YAML.

### Manual smoke test (post-merge, in a real cluster)

Documented in the implementation plan, not the spec — but in scope:

1. Deploy with `backup.enabled=true`, IRSA mode.
2. Have agent generate session data, confirm preStop sync to S3.
3. Delete pod (`kubectl delete pod …`).
4. New pod starts → confirm `/home/agent` is fully populated **before**
   `quill` process emits its first log line (check kubelet event timeline).
5. **Verify ownership**: `kubectl exec <pod> -c quill -- stat -c '%u:%g %n' /home/agent/.local/share/kiro-cli/data.sqlite3` returns `1000:1000 …`.
6. **Verify writability**: same exec runs `touch /home/agent/.write-probe && rm /home/agent/.write-probe` without error.
7. `/pick` from a chat client returns the historical session list.

## Risks and trade-offs

- **CrashLoop on first deploy if S3 path malformed**: removing `|| true`
  trades silent data loss for loud failure. Considered correct: the chart
  user explicitly opted into backup; misconfigured backup should not
  silently degrade to no-backup.
- **Two containers share image pull**: `rclone/rclone:1.66` is pulled
  once per node; both containers share the layer. Negligible cost.
- **Sidecar idle resource usage doubled**: trivially small (`sleep
  infinity` consumes ~1MB RSS). Existing `resources` block still applies
  to both, so the requests/limits double in `kubectl describe pod`
  arithmetic; for the default values (10m / 32Mi requests) that's still
  immaterial relative to the quill container.
- **Helper template indentation foot-gun**: callers must pick the right
  `nindent` depth. Mitigated by tests #6 and #8 above (rendered output
  must contain specific env-var keys at the right path).

## Migration

Existing chart users running 2026-04-27's single-sidecar layout: a
`helm upgrade` replaces the Deployment template; rolling update creates
new pods with the two-container init layout. No state migration needed
because the S3 prefix and emptyDir contracts are unchanged.

**Existing root-owned files in S3**: any user already running the broken
v0.22.x layout has a snapshot in S3 that may have been written by either
the agent (UID 1000, mode 0644) or never written at all (sidecar wrote
with UID 0 to the emptyDir, then preStop sync uploaded those root-owned
metadata to S3). After upgrade:

- `rclone copy` from S3 to the emptyDir runs as UID 1000:1000 — files
  land owned by UID 1000:1000 regardless of how they were uploaded.
- Pod-level `fsGroup: 1000` chowns the emptyDir's existing tree as
  belt-and-suspenders.

So no manual cleanup of S3 objects is required. The first post-upgrade
pod start auto-heals ownership.
