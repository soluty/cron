package httpManagement

import (
	"bytes"
	"github.com/alaingilbert/cron"
	"github.com/alaingilbert/cron/internal/utils"
	"net/http"
	"strconv"
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

func getIndexHandler(c *cron.Cron) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		jobRuns := c.RunningJobs()
		entries := c.Entries()
		b.WriteString(css + `

` + getMenu() + `
Running jobs (` + strconv.Itoa(len(jobRuns)) + `)<br />
<table>
	<thead>
		<th>Entry ID</th>
		<th>Run ID</th>
		<th>Label</th>
		<th>Started at</th>
		<th>Actions</th>
	</thead>
	<tbody>
`)
		for _, jobRun := range jobRuns {
			b.WriteString(`
	<tr>
		<td><span class="monospace"><a href="/entries/` + string(jobRun.Entry.ID) + `">` + string(jobRun.Entry.ID) + `</a></span></td>
		<td><span class="monospace"><a href="/entries/` + string(jobRun.Entry.ID) + `/runs/` + string(jobRun.RunID) + `">` + string(jobRun.RunID) + `</a></span></td>
		<td>` + jobRun.Entry.Label + `</td>
		<td>` + jobRun.StartedAt.Format(time.DateTime) + `</td>
		<td>
			<form method="POST" class="d-inline-block">
				<input type="hidden" name="formName" value="cancelRun" />
				<input type="hidden" name="entryID" value="` + string(jobRun.Entry.ID) + `" />
				<input type="hidden" name="runID" value="` + string(jobRun.RunID) + `" />
				<input type="submit" value="Cancel" />
			</form>
		</td>
	</tr>`)
		}
		b.WriteString(`
	</tbody>
</table>

<hr />

Entries (` + strconv.Itoa(len(entries)) + `)<br />

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
`)
		for _, entry := range entries {
			b.WriteString(`
<tr>
	<td><span class="monospace"><a href="/entries/` + string(entry.ID) + `">` + string(entry.ID) + `</a></span></td>
	<td>` + entry.Label + `</td>
	<td>` + entry.Prev.Format(time.DateTime) + `</td>
	<td>` + entry.Next.Format(time.DateTime) + `</td>
	<td>` + utils.Ternary(entry.Active, `<span class="success">T</span>`, `<span class="danger">F</span>`) + `</td>
	<td>
		<form method="POST" class="d-inline-block">
			<input type="hidden" name="formName" value="runNow" />
			<input type="hidden" name="entryID" value="` + string(entry.ID) + `" />
			<input type="submit" value="Run now" />
		</form>
`)
			if entry.Active {
				b.WriteString(`
		<form method="POST" class="d-inline-block">
			<input type="hidden" name="formName" value="disableEntry" />
			<input type="hidden" name="entryID" value="` + string(entry.ID) + `" />
			<input type="submit" value="Disable" />
		</form>
`)
			} else {
				b.WriteString(`
		<form method="POST" class="d-inline-block">
			<input type="hidden" name="formName" value="enableEntry" />
			<input type="hidden" name="entryID" value="` + string(entry.ID) + `" />
			<input type="submit" value="Enable" />
		</form>
`)
			}
			b.WriteString(`
		<form method="POST" class="d-inline-block">
			<input type="hidden" name="formName" value="removeEntry" />
			<input type="hidden" name="entryID" value="` + string(entry.ID) + `" />
			<input type="submit" value="Remove" />
		</form>
	</td>
</tr>
`)
		}
		b.WriteString(`
	</tbody>
</table>
`)
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
		b.WriteString(css + `
` + getMenu() + `
<h1>` + string(entry.ID) + `</h1>
Active: ` + utils.Ternary(entry.Active, `<span class="success">T</span>`, `<span class="danger">F</span>`) + `<br />
<hr />
<div>
	<label for="label">Job label:</label>
	<form method="POST">
		<input type="hidden" name="formName" value="updateLabel" />
		<input type="text" name="label" value="` + entry.Label + `" placeholder="Label" />
		<input type="submit" value="Update label" id="label" />
	</form>
</div>
<hr />
Running jobs (` + strconv.Itoa(len(jobRuns)) + `)<br />
<table>
	<thead>
		<th>Entry ID</th>
		<th>Run ID</th>
		<th>Label</th>
		<th>Started at</th>
		<th>Actions</th>
	</thead>
	<tbody>
`)
		for _, jobRun := range jobRuns {
			b.WriteString(`
	<tr>
		<td><span class="monospace"><a href="/entries/` + string(jobRun.Entry.ID) + `">` + string(jobRun.Entry.ID) + `</a></span></td>
		<td><span class="monospace"><a href="/entries/` + string(jobRun.Entry.ID) + `/runs/` + string(jobRun.RunID) + `">` + string(jobRun.RunID) + `</a></span></td>
		<td>` + jobRun.Entry.Label + `</td>
		<td>` + jobRun.StartedAt.Format(time.DateTime) + `</td>
		<td>
			<form method="POST" class="d-inline-block">
				<input type="hidden" name="formName" value="cancelRun" />
				<input type="hidden" name="entryID" value="` + string(jobRun.Entry.ID) + `" />
				<input type="hidden" name="runID" value="` + string(jobRun.RunID) + `" />
				<input type="submit" value="Cancel" />
			</form>
		</td>
	</tr>`)
		}
		b.WriteString(`
	</tbody>
</table>

Completed jobs<br />
<table>
	<thead>
		<th>Entry ID</th>
		<th>Run ID</th>
		<th>Label</th>
		<th>Started at</th>
		<th>Error</th>
		<th>Panic</th>
	</thead>
	<tbody>
`)
		for _, jobRun := range completedJobRuns {
			b.WriteString(`
	<tr>
		<td><span class="monospace"><a href="/entries/` + string(jobRun.Entry.ID) + `">` + string(jobRun.Entry.ID) + `</a></span></td>
		<td><span class="monospace"><a href="/entries/` + string(jobRun.Entry.ID) + `/runs/` + string(jobRun.RunID) + `">` + string(jobRun.RunID) + `</a></span></td>
		<td>` + jobRun.Entry.Label + `</td>
		<td>` + jobRun.StartedAt.Format(time.DateTime) + `</td>
		<td>`)
			if jobRun.Error != nil {
				b.WriteString(`<span class="danger" title="` + jobRun.Error.Error() + `">Error</span>`)
			} else {
				b.WriteString(`<span class="success">-</span>`)
			}
			b.WriteString(`
		</td>
		<td>`)
			if jobRun.Panic {
				b.WriteString(`<span class="danger">Panic</span>`)
			} else {
				b.WriteString(`<span class="success">-</span>`)
			}
			b.WriteString(`
		</td>
	</tr>`)
		}
		b.WriteString(`
	</tbody>
</table>
`)
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
		b.WriteString(css + `
` + getMenu() + `
<h1>Run: ` + string(jobRun.RunID) + `</h1>
<div class="mb-1">
	Entry: <span class="monospace"><a href="/entries/` + string(entry.ID) + `">` + string(entry.ID) + `</a></span><br />
</div>
<form method="POST">
	<input type="hidden" name="formName" value="cancelRun" />
	<input type="hidden" name="entryID" value="` + string(jobRun.Entry.ID) + `" />
	<input type="hidden" name="runID" value="` + string(jobRun.RunID) + `" />
	<input type="submit" value="Cancel" />
</form>
<hr />
Events:<br />
<table>
	<thead>
		<tr><th>Type</th><th>Created at</th></tr>
	</thead>
	<tbody>
`)
		for _, evt := range jobRun.Events {
			b.WriteString(`
	<tr><td>` + evt.Typ.String() + `</td><td>` + evt.CreatedAt.Format(time.DateTime) + `</td></tr>
`)
		}
		b.WriteString(`
	</tbody>
</table>
`)
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
