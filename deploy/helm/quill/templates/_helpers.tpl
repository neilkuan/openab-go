{{- define "quill.name" -}}
{{ .Chart.Name }}
{{- end }}

{{- define "quill.fullname" -}}
{{ .Release.Name }}-{{ .Chart.Name }}
{{- end }}

{{- define "quill.labels" -}}
app.kubernetes.io/name: {{ include "quill.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "quill.selectorLabels" -}}
app.kubernetes.io/name: {{ include "quill.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
S3 sync env vars shared by both rclone init/sidecar containers.
Caller dict:
  ctx  — the root context (`$`) for fullname helper
  name — the instance name (e.g. "kiro")
  b    — the merged backup config (`merge $inst.backup $.Values.backup`)
*/}}
{{- define "quill.s3.envVars" -}}
- name: HOME
  value: /tmp
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
