# {{ .Code }}

These are all the exams for {{ .Code }}.

See `handin` at https://fn.lc/hw/{{ .Code }}.

{{ range $key, $value := .Years }}
{{ if eq $key 0 }}
## Undated
{{ else }}
## {{ $key }}
{{ end }}
{{ range $file := $value.Files }}
* [{{ $file.Name }}](/{{ $file.Path}})
{{ end }}
{{ end }}
