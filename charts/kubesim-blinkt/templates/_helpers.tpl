{{/* vim: set filetype=mustache: */}}
{{- define "kubesim_blinkt.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "kubesim_blinkt.fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "kubesim_blinkt.labels" -}}
app: {{ template "kubesim_blinkt.name" . }}
chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
release: {{ .Release.Name }}
heritage: {{ .Release.Service }}
{{- end -}}

{{/* ResourceClaimTemplate specs are immutable: the name carries a hash of the
     LED values, so a change creates a new template (and new pods). */}}
{{- define "kubesim_blinkt.claimTemplate" -}}
{{ template "kubesim_blinkt.fullname" . }}-{{ .Values.blinkt | toJson | sha256sum | trunc 8 }}
{{- end -}}

{{- define "kubesim_blinkt.validate" -}}
{{- $mode := .Values.blinkt.mode -}}
{{- if not (has $mode (list "agent" "cdi" "legacy")) -}}
{{- fail (printf "blinkt.mode must be agent, cdi or legacy, not %q" $mode) -}}
{{- end -}}
{{- if not .Values.blinkt.pixels -}}
{{- fail "blinkt.pixels must list at least one LED" -}}
{{- end -}}
{{- if not (has .Values.kind (list "Deployment" "DaemonSet")) -}}
{{- fail (printf "kind must be Deployment or DaemonSet, not %q" .Values.kind) -}}
{{- end -}}
{{- range $i, $p := .Values.blinkt.pixels -}}
{{- if hasKey $p "index" -}}
{{- if or (lt (int $p.index) 0) (gt (int $p.index) 7) -}}
{{- fail (printf "blinkt.pixels[%d].index must be 0-7" $i) -}}
{{- end -}}
{{- else if eq $mode "legacy" -}}
{{- fail (printf "blinkt.pixels[%d]: legacy mode needs an index (any-free LEDs need DRA)" $i) -}}
{{- end -}}
{{- if or (not $p.color) (ne (len $p.color) 3) -}}
{{- fail (printf "blinkt.pixels[%d].color must be [r, g, b]" $i) -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "kubesim_blinkt.hasIndexed" -}}
{{- range .Values.blinkt.pixels }}{{ if hasKey . "index" }}yes{{ end }}{{ end -}}
{{- end -}}
