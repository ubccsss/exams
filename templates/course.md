<style>
th:first-child { width: 100000px; }
</style>

# {{ .Code }}
{{ if ne (len .Desc) 0 }}
<p><strong>Description:</strong> {{.Desc}}</p>
{{end}}

{{ if ne (len .Years) 0 }}
These are all the exams for {{ .Code }}.
{{ else }}
Sorry, we don't have any exams for {{ .Code }}. Please upload some below!
{{ end }}

{{ $years := .Years}}
{{ range $key, $year := .YearSections }}
{{ $files := index $years $year }}
{{ if ne (len $files) 0 }}
{{ if eq $year 0 }}
## Undated
{{ else }}
## {{ $year }}
{{ end }}
| File | Term |
|------|------|
{{ range $file := $files -}}
|[{{ $file.Name }}]({{ $file.Path | pathToURL }})|{{ $file.Term }}|
{{ end }}
{{ end }}
{{ end }}

{{ if ne (len .PotentialFiles) 0 }}
## Other Possible Files

These are files that we've automatically discovered and think might be exams but
haven't gotten around to manually indexing them. These labels have been
automatically assigned using machine learning.

{{ $names := .FileNames }}

{{ if ne (len .CompletedML) 0 }}
| File | Type | Year | Term |
|------|------|------|------|
{{ range $file := .CompletedML -}}
{{- if $file.Inferred -}}
|[{{ index $names $file.Hash }}]({{ $file.Path | pathToURL }}) |
{{- $file.Inferred.Name -}}
| {{ $file.Inferred.Year -}}
| {{ $file.Inferred.Term }}|
{{ end -}}
{{ end }}
{{ end }}

{{ if ne (len .PendingML) 0 }}
### Pending Machine Learning Labels

{{ range $file := .PendingML -}}
{{- if not $file.Inferred -}}
* {{ $file.Source }}
{{end -}}
{{end}}
{{ end }}

{{ end }}

## Upload

<style>input#shouldbeempty{display:none;}</style>
<form method="POST" action="/upload?course={{.Code}}" enctype="multipart/form-data">
  <div class="form-group">
    <label for="name">File Type</label>
    <br>
    <select id="name" name="name" size="16">
      <option>Final</option>
      <option>Final (Solution)</option>
      <option>Sample Final</option>
      <option>Sample Final (Solution)</option>
      <option>Midterm</option>
      <option>Midterm (Solution)</option>
      <option>Sample Midterm</option>
      <option>Sample Midterm (Solution)</option>
      <option>Midterm 1</option>
      <option>Midterm 1 (Solution)</option>
      <option>Sample Midterm 1</option>
      <option>Sample Midterm 1 (Solution)</option>
      <option>Midterm 2</option>
      <option>Midterm 2 (Solution)</option>
      <option>Sample Midterm 2</option>
      <option>Sample Midterm 2 (Solution)</option>
    </select>
  </div>
  <div class="form-group">
    <label for="year">Year</label>
    <p class="help-block">Year is the year that happens during Term 1. A final
    from Sep 2016 or Jan 2017 would be 2016.</p>
    <input type="number" class="form-control" id="year" name="year" placeholder="Year">
  </div>
  <div class="form-group">
    <label for="term">Term</label>
    <br>
    <select id="term" size="3" name="term">
      <option>W1</option>
      <option>W2</option>
      <option>S</option>
    </select>
  </div>
  <div class="form-group">
    <label for="exam">File</label>
    <input type="file" id="exam" name="exam">
  </div>
  <input type="text" id="shouldbeempty" name="shouldbeempty">
  <button type="submit" class="btn btn-default">Upload</button>
</form>


## Other Resources

See `handin` at https://fn.lc/hw/{{ .Code }}.

