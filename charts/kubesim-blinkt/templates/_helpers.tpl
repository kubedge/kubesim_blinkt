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

{{/* Release colours, and the fixed LED each one uses in legacy mode. */}}
{{- define "kubesim_blinkt.releaseColor" -}}
{{- $c := dict "red" (list 255 0 0) "green" (list 0 255 0) "blue" (list 0 0 255) -}}
{{- get $c .Values.blinkt.release | toJson -}}
{{- end -}}
{{- define "kubesim_blinkt.releaseIndex" -}}
{{- get (dict "red" 0 "green" 1 "blue" 2) .Values.blinkt.release -}}
{{- end -}}

{{/* The LEDs one pod lights, as JSON: blinkt.pixels, or one LED in the release
     colour (any free LED in DRA modes, the release's fixed LED in legacy mode). */}}
{{- define "kubesim_blinkt.pixels" -}}
{{- if .Values.blinkt.pixels -}}
{{- toJson .Values.blinkt.pixels -}}
{{- else if eq .Values.blinkt.mode "legacy" -}}
{{- toJson (list (dict "index" (int (include "kubesim_blinkt.releaseIndex" .)) "color" (include "kubesim_blinkt.releaseColor" . | fromJsonArray))) -}}
{{- else -}}
{{- toJson (list (dict "color" (include "kubesim_blinkt.releaseColor" . | fromJsonArray))) -}}
{{- end -}}
{{- end -}}

{{/* "yes" when any LED is fixed (has an index). */}}
{{- define "kubesim_blinkt.hasIndexed" -}}
{{- range (include "kubesim_blinkt.pixels" . | fromJsonArray) }}{{ if hasKey . "index" }}yes{{ end }}{{ end -}}
{{- end -}}

{{- define "kubesim_blinkt.validate" -}}
{{- $mode := .Values.blinkt.mode -}}
{{- if not (has $mode (list "agent" "cdi" "legacy")) -}}
{{- fail (printf "blinkt.mode must be agent, cdi or legacy, not %q" $mode) -}}
{{- end -}}
{{- if and (not .Values.blinkt.pixels) (not (has .Values.blinkt.release (list "red" "green" "blue"))) -}}
{{- fail (printf "blinkt.release must be red, green or blue, not %q" .Values.blinkt.release) -}}
{{- end -}}
{{- if not (has .Values.kind (list "Deployment" "DaemonSet")) -}}
{{- fail (printf "kind must be Deployment or DaemonSet, not %q" .Values.kind) -}}
{{- end -}}
{{- range $i, $p := (include "kubesim_blinkt.pixels" . | fromJsonArray) -}}
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
