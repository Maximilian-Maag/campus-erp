{{/* Common name/label helpers. */}}

{{- define "campus-erp.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "campus-erp.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "campus-erp.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "campus-erp.selectorLabels" -}}
app.kubernetes.io/name: {{ include "campus-erp.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "campus-erp.labels" -}}
helm.sh/chart: {{ include "campus-erp.chart" . }}
{{ include "campus-erp.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/* name for the component-level objects, e.g. campus-erp-api */}}
{{- define "campus-erp.componentName" -}}
{{- printf "%s-%s" (include "campus-erp.fullname" .) .component | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "campus-erp.componentSelectorLabels" -}}
{{ include "campus-erp.selectorLabels" . }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "campus-erp.backendImage" -}}
{{- printf "%s:%s" .Values.images.backend.repository (.Values.images.backend.tag | toString) -}}
{{- end -}}

{{- define "campus-erp.frontendImage" -}}
{{- printf "%s:%s" .Values.images.frontend.repository (.Values.images.frontend.tag | toString) -}}
{{- end -}}

{{/* Secret that carries the tenant DSN. */}}
{{- define "campus-erp.dbSecretName" -}}
{{- if .Values.database.existingSecret -}}
{{- .Values.database.existingSecret -}}
{{- else -}}
{{- printf "%s-db" (include "campus-erp.fullname" .) -}}
{{- end -}}
{{- end -}}
