# Exams Database


### Please send any exams you have to [vpc@ubccsss.org](mailto:vpc@ubccsss.org)!

You can find our collection of CPSC exams and quizzes here. They’re sorted by year and course. Solutions (where they exist) are also provided.

*NOTE:* These exams are here as reference ONLY. Examinable materials and course content vary from year to year, so any materials on this website might be out of date. We are not responsible for any mistakes in the solution materials provided herein; however, we will accept notifications as such so we can place appropriate notices.

{{ range $level, $courses := . }}
## {{$level}}
{{ range $cid, $c := $courses }}
* [{{$c.Name}} {{ if ne $c.FileCount 0 }}({{$c.FileCount}}){{end}}](./{{$cid}}/) {{end}}
{{ end }}
