{{- define "xenorchestra-csi-driver.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "xenorchestra-csi-driver.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}{{ .Release.Name | trunc 63 | trimSuffix "-" }}{{ else }}{{ printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}{{ end }}
{{- end }}
{{- end }}

{{- define "xenorchestra-csi-driver.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "xenorchestra-csi-driver.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: xenorchestra-csi-driver
{{- end }}

{{- define "xenorchestra-csi-driver.controllerServiceAccountName" -}}
{{- if .Values.serviceAccount.create }}{{ default (printf "%s-controller" (include "xenorchestra-csi-driver.fullname" .)) .Values.serviceAccount.controllerName }}{{ else }}{{ default "default" .Values.serviceAccount.controllerName }}{{ end }}
{{- end }}

{{- define "xenorchestra-csi-driver.nodeServiceAccountName" -}}
{{- if .Values.serviceAccount.create }}{{ default (printf "%s-node" (include "xenorchestra-csi-driver.fullname" .)) .Values.serviceAccount.nodeName }}{{ else }}{{ default "default" .Values.serviceAccount.nodeName }}{{ end }}
{{- end }}

{{- define "xenorchestra-csi-driver.configSecretName" -}}
{{- default (include "xenorchestra-csi-driver.fullname" .) .Values.existingConfigSecret }}
{{- end }}

{{- define "xenorchestra-csi-driver.image" -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) }}
{{- end }}
