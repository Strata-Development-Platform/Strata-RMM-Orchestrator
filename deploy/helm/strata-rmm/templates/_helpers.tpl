{{- define "strata-rmm.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "strata-rmm.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "strata-rmm.labels" -}}
helm.sh/chart: {{ include "strata-rmm.name" . }}-{{ .Chart.Version | replace "+" "_" }}
{{ include "strata-rmm.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "strata-rmm.selectorLabels" -}}
app.kubernetes.io/name: {{ include "strata-rmm.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "strata-rmmm.serviceAccountName" -}}
{{- if .Values.orchestrator.serviceAccount.create }}
{{- default (include "strata-rmm.fullname" .) .Values.orchestrator.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.orchestrator.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "strata-rmm.nats-url" -}}
{{- if .Values.nats.enabled }}
{{- if .Values.nats.cluster.enabled }}
nats://nats.{{ .Release.Namespace }}.svc:4222
{{- else }}
nats://{{ .Release.Name }}-nats:4222
{{- end }}
{{- else }}
{{- .Values.orchestrator.nats.url }}
{{- end }}
{{- end }}

{{- define "strata-rmm.timescale-dsn" -}}
{{- if .Values.timescaledb.enabled }}
postgres://{{ .Values.timescaledb.credentials.user }}:{{ .Values.timescaledb.credentials.password }}@timescaledb:5432/{{ .Values.timescaledb.credentials.database }}?sslmode=disable
{{- else if .Values.postgresql.enabled }}
postgres://{{ .Values.postgresql.auth.username }}:{{ .Values.postgresql.auth.password }}-postgres:5432/{{ .Values.postgresql.auth.database }}?sslmode=disable
{{- else }}
{{- .Values.orchestrator.timescale.dsn }}
{{- end }}
{{- end }}

{{- define "strata-rmm.image" -}}
{{- $registry := .Values.global.imageRegistry | default .Values.orchestrator.image.repository }}
{{- $tag := .Values.orchestrator.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" $registry $tag }}
{{- end }}
