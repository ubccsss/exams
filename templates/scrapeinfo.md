<title>Scrape Info</title>

# Scrape Info

Files: {{.Files}}

Pages pending fetch: {{.Pending}}

Total known URLs: {{.Seen}}

## Content Types

|Content Type|Count|
|---|---|
{{- range .Types }}
|{{.Type}}|{{.Count}}|
{{- end}}
