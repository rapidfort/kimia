{{- define "kimia-builds.fullname" -}}
{{- if .Values.nameOverride -}}
{{ .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- else -}}
{{- printf "%s" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end }}

{{- define "kimia-builds.registriesConf" -}}
unqualified-search-registries = [{{- range $i, $r := .Values.global.registries.unqualifiedSearch -}}{{ if gt $i 0 }}, {{ end }}"{{ $r }}"{{- end -}}]

{{- range .Values.global.registries.entries }}
[[registry]]
location = "{{ .location }}"
{{- if hasKey . "insecure" }}
insecure = {{ .insecure }}
{{- end }}
{{- if hasKey . "blocked" }}
blocked = {{ .blocked }}
{{- end }}

{{- end }}
{{- end }}

{{- define "debug.comment" -}}
# DEBUG {{ .name }}: {{ toJson .value }}
{{- end -}}