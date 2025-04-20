package httpManagement

import (
	"bytes"
	"github.com/alaingilbert/cron"
	"html/template"
	"net/http"
	"time"
)

func GetMux(c *cron.Cron) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", getIndexHandler(c))
	mux.HandleFunc("POST /{$}", postIndexHandler(c))
	mux.HandleFunc("GET /entries/{entryID}/{$}", getEntryHandler(c))
	mux.HandleFunc("POST /entries/{entryID}/{$}", postEntryHandler(c))
	mux.HandleFunc("GET /entries/{entryID}/runs/{runID}/{$}", getRunHandler(c))
	mux.HandleFunc("POST /entries/{entryID}/runs/{runID}/{$}", postRunHandler(c))
	return mux
}

var css = `
<style>
	html {
		background-color: #222;
		color: #eee;
		font-family: -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif,"Apple Color Emoji","Segoe UI Emoji","Segoe UI Symbol";
	}
	form {
		margin: 0;
	}
	table td {
		padding: 0 5px;
	}
	.monospace { font-family: monospace; }
	a {
		color: #eee;
	}
	.d-inline-block { display: inline-block; }
	.success { color: #0f0; }
	.danger { color: #f00; }
	.mb-1 { margin-bottom: 10px; }
</style>
`

func getMenu() string {
	return `
<a href="/">home</a>
<hr />
Current time: ` + time.Now().Format(time.DateTime) + `<br />
<hr />`
}

var funcsMap = template.FuncMap{
	"FmtDate": func(t time.Time) string { return t.Format(time.DateTime) },
}

func getIndexHandler(c *cron.Cron) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		jobRuns := c.RunningJobs()
		entries := c.Entries()

		tmpl, _ := template.New("").Funcs(funcsMap).Parse(`
<!doctype html>
<html>
<head>
	` + css + `
</head>
<body>
` + getMenu() + `
Running jobs ({{ len .JobRuns }})<br />
<table>
	<thead>
		<th>Entry ID</th>
		<th>Run ID</th>
		<th>Label</th>
		<th>Started at</th>
		<th>Actions</th>
	</thead>
	<tbody>
		{{ range .JobRuns }}
			<tr>
				<td><span class="monospace"><a href="/entries/{{ .Entry.ID }}">{{ .Entry.ID }}</a></span></td>
				<td><span class="monospace"><a href="/entries/{{ .Entry.ID }}/runs/{{ .RunID }}">{{ .RunID }}</a></span></td>
				<td>{{ .Entry.Label }}</td>
				<td>{{ .StartedAt | FmtDate }}</td>
				<td>
					<form method="POST" class="d-inline-block">
						<input type="hidden" name="formName" value="cancelRun" />
						<input type="hidden" name="entryID" value="{{ .Entry.ID }}" />
						<input type="hidden" name="runID" value="{{ .RunID }}" />
						<input type="submit" value="Cancel" />
					</form>
				</td>
			</tr>
		{{ else }}
			<tr><td colspan="5"><em>No running jobs</em></td></tr>
		{{ end }}
	</tbody>
</table>


<hr />

Entries ({{ len .Entries }})<br />

<table>
	<thead>
		<tr>
			<th>Entry ID</th>
			<th>Label</th>
			<th>Prev</th>
			<th>Next</th>
			<th>Active</th>
			<th>Actions</th>
		</tr>
	</thead>
	<tbody>
		{{ range .Entries }}
			<tr>
				<td><span class="monospace"><a href="/entries/{{ .ID }}">{{ .ID }}</a></span></td>
				<td>{{ .Label }}</td>
				<td>{{ .Prev | FmtDate }}</td>
				<td>{{ .Next | FmtDate }}</td>
				<td>
					{{ if .Active }}
						<span class="success">T</span>
					{{ else }}
						<span class="danger">F</span>
					{{ end }}
				</td>
				<td>
					<form method="POST" class="d-inline-block">
						<input type="hidden" name="formName" value="runNow" />
						<input type="hidden" name="entryID" value="{{ .ID }}" />
						<input type="submit" value="Run now" />
					</form>
					{{ if .Active }}
						<form method="POST" class="d-inline-block">
							<input type="hidden" name="formName" value="disableEntry" />
							<input type="hidden" name="entryID" value="{{ .ID }}" />
							<input type="submit" value="Disable" />
						</form>
					{{ else }}
						<form method="POST" class="d-inline-block">
							<input type="hidden" name="formName" value="enableEntry" />
							<input type="hidden" name="entryID" value="{{ .ID }}" />
							<input type="submit" value="Enable" />
						</form>
					{{ end }}
					<form method="POST" class="d-inline-block">
						<input type="hidden" name="formName" value="removeEntry" />
						<input type="hidden" name="entryID" value="{{ .ID }}" />
						<input type="submit" value="Remove" />
					</form>
				</td>
			</tr>
		{{ else }}
			<tr><td colspan="6"><em>No entries</em></td></tr>
		{{ end }}
	</tbody>
</table>
</body>
</html>
`)
		_ = tmpl.Execute(&b, map[string]any{
			"JobRuns": jobRuns,
			"Entries": entries,
		})
		_, _ = w.Write(b.Bytes())
	}
}

func postIndexHandler(c *cron.Cron) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		formName := r.PostFormValue("formName")
		if formName == "cancelRun" {
			entryID := cron.EntryID(r.PostFormValue("entryID"))
			runID := cron.RunID(r.PostFormValue("runID"))
			c.CancelRun(entryID, runID)
		} else if formName == "removeEntry" {
			entryID := cron.EntryID(r.PostFormValue("entryID"))
			c.Remove(entryID)
		} else if formName == "enableEntry" {
			entryID := cron.EntryID(r.PostFormValue("entryID"))
			c.Enable(entryID)
		} else if formName == "disableEntry" {
			entryID := cron.EntryID(r.PostFormValue("entryID"))
			c.Disable(entryID)
		} else if formName == "runNow" {
			entryID := cron.EntryID(r.PostFormValue("entryID"))
			c.RunNow(entryID)
		}
		w.Header().Set("Location", "/")
		w.WriteHeader(http.StatusSeeOther)
	}
}

func getEntryHandler(c *cron.Cron) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entryID := cron.EntryID(r.PathValue("entryID"))
		entry, err := c.Entry(entryID)
		if err != nil {
			w.Header().Set("Location", "/")
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		var b bytes.Buffer
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		jobRuns := c.RunningJobsFor(entryID)
		completedJobRuns := c.CompletedJobRunsFor(entryID)
		tmpl, _ := template.New("").Funcs(funcsMap).Parse(`
<!doctype html>
<html>
<head>
	` + css + `
</head>
<body>
` + getMenu() + `
<h1>{{ .Entry.ID }}</h1>
<div>Label: {{ .Entry.Label }}</div>
<div>Active: {{ if .Entry.Active }}<span class="success">T</span>{{ else }}<span class="danger">F</span>{{ end }}</div>
<hr />
<div class="mb-1">
{{ if .Entry.Active }}
		<form method="POST" class="d-inline-block">
			<input type="hidden" name="formName" value="disableEntry" />
			<input type="hidden" name="entryID" value="{{ .Entry.ID }}" />
			<input type="submit" value="Disable" />
		</form>
{{ else }}
		<form method="POST" class="d-inline-block">
			<input type="hidden" name="formName" value="enableEntry" />
			<input type="hidden" name="entryID" value="{{ .Entry.ID }}" />
			<input type="submit" value="Enable" />
		</form>
{{ end }}
</div>

<div>
	<label for="label">Job label:</label>
	<form method="POST">
		<input type="hidden" name="formName" value="updateLabel" />
		<input type="text" name="label" value="{{ .Entry.Label }}" placeholder="Label" />
		<input type="submit" value="Update label" id="label" />
	</form>
</div>
<hr />
Running jobs ({{ len .JobRuns }})<br />
<table class="mb-1">
	<thead>
		<th>Entry ID</th>
		<th>Run ID</th>
		<th>Label</th>
		<th>Started at</th>
		<th>Actions</th>
	</thead>
	<tbody>
		{{ range .JobRuns }}
			<tr>
				<td><span class="monospace"><a href="/entries/{{ .Entry.ID }}">{{ .Entry.ID }}</a></span></td>
				<td><span class="monospace"><a href="/entries/{{ .Entry.ID }}/runs/{{ .RunID }}">{{ .RunID }}</a></span></td>
				<td>{{ .Entry.Label }}</td>
				<td>{{ .StartedAt | FmtDate }}</td>
				<td>
					<form method="POST" class="d-inline-block">
						<input type="hidden" name="formName" value="cancelRun" />
						<input type="hidden" name="entryID" value="{{ .Entry.ID }}" />
						<input type="hidden" name="runID" value="{{ .RunID }}" />
						<input type="submit" value="Cancel" />
					</form>
				</td>
			</tr>
		{{ end }}
	</tbody>
</table>

Completed jobs ({{ len .CompletedJobRuns }})<br />
<table class="mb-1">
	<thead>
		<th>Entry ID</th>
		<th>Run ID</th>
		<th>Label</th>
		<th>Started at</th>
		<th>Completed at</th>
		<th>Error</th>
		<th>Panic</th>
	</thead>
	<tbody>
		{{ range .CompletedJobRuns }}
			<tr>
				<td><span class="monospace"><a href="/entries/{{ .Entry.ID }}">{{ .Entry.ID }}</a></span></td>
				<td><span class="monospace"><a href="/entries/{{ .Entry.ID }}/runs/{{ .RunID }}">{{ .RunID }}</a></span></td>
				<td>{{ .Entry.Label }}</td>
				<td>{{ .StartedAt | FmtDate }}</td>
				<td>{{ .CompletedAt | FmtDate }}</td>
				<td>
					{{ if .Error }}
						<span class="danger" title="{{ .Error.Error }}">Error</span>
					{{ else }}
						<span>-</span>
					{{ end }}
				</td>
				<td>
					{{ if .Panic }}
						<span class="danger">Panic</span>
					{{ else }}
						<span>-</span>
					{{ end }}
				</td>
			</tr>
		{{ end }}
	</tbody>
</table>

</body>
</html>
`)
		_ = tmpl.Execute(&b, map[string]any{
			"Entry":            entry,
			"JobRuns":          jobRuns,
			"CompletedJobRuns": completedJobRuns,
		})
		_, _ = w.Write(b.Bytes())
	}
}

func postEntryHandler(c *cron.Cron) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entryID := cron.EntryID(r.PathValue("entryID"))
		_, err := c.Entry(entryID)
		if err != nil {
			w.Header().Set("Location", "/")
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		formName := r.PostFormValue("formName")
		if formName == "updateLabel" {
			label := r.PostFormValue("label")
			c.UpdateLabel(entryID, label)
		} else if formName == "enableEntry" {
			entryID := cron.EntryID(r.PostFormValue("entryID"))
			c.Enable(entryID)
		} else if formName == "disableEntry" {
			entryID := cron.EntryID(r.PostFormValue("entryID"))
			c.Disable(entryID)
		} else if formName == "cancelRun" {
			entryID := cron.EntryID(r.PostFormValue("entryID"))
			runID := cron.RunID(r.PostFormValue("runID"))
			c.CancelRun(entryID, runID)
		}
		w.Header().Set("Location", "/entries/"+string(entryID))
		w.WriteHeader(http.StatusSeeOther)
	}
}

func getRunHandler(c *cron.Cron) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entryID := cron.EntryID(r.PathValue("entryID"))
		runID := cron.RunID(r.PathValue("runID"))
		entry, err := c.Entry(entryID)
		if err != nil {
			w.Header().Set("Location", "/")
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		jobRun, err := c.GetRun(entryID, runID)
		if err != nil {
			w.Header().Set("Location", "/")
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		var b bytes.Buffer
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		tmpl, _ := template.New("").Funcs(funcsMap).Parse(`
<!doctype html>
<html>
<head>
	` + css + `
</head>
<body>
` + getMenu() + `
<h1>Run: {{ .JobRun.RunID }}</h1>
<div class="mb-1">
	Entry: <span class="monospace"><a href="/entries/{{ .Entry.ID }}">{{ .Entry.ID }}</a></span><br />
</div>
<form method="POST">
	<input type="hidden" name="formName" value="cancelRun" />
	<input type="hidden" name="entryID" value="{{ .JobRun.Entry.ID }}" />
	<input type="hidden" name="runID" value="{{ .JobRun.RunID }}" />
	<input type="submit" value="Cancel" />
</form>
<hr />
Events:<br />
<table>
	<thead>
		<tr><th>Type</th><th>Created at</th></tr>
	</thead>
	<tbody>
		{{ range .JobRun.Events }}
			<tr><td>{{ .Typ }}</td><td>{{ .CreatedAt | FmtDate }}</td></tr>
		{{ end }}
	</tbody>
</table>
</body>
</html>
`)
		_ = tmpl.Execute(&b, map[string]any{
			"JobRun": jobRun,
			"Entry":  entry,
		})
		_, _ = w.Write(b.Bytes())
	}
}

func postRunHandler(c *cron.Cron) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entryID := cron.EntryID(r.PathValue("entryID"))
		runID := cron.RunID(r.PathValue("runID"))
		entry, err := c.Entry(entryID)
		if err != nil {
			w.Header().Set("Location", "/")
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		jobRun, err := c.GetRun(entryID, runID)
		if err != nil {
			w.Header().Set("Location", "/")
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		formName := r.PostFormValue("formName")
		if formName == "cancelRun" {
			c.CancelRun(entry.ID, jobRun.RunID)
			w.Header().Set("Location", "/")
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		w.Header().Set("Location", "/entries/"+string(entryID))
		w.WriteHeader(http.StatusSeeOther)
	}
}
