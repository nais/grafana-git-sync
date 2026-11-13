{{- define "grafana-git-sync.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Values.image.tag | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end }}

{{- define "grafana-git-sync.serviceAccountName" -}}
{{- default .Release.Name .Values.serviceAccount.name }}
{{- end }}
