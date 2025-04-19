package httpManagement

import (
	"bytes"
	"github.com/alaingilbert/cron"
	"net/http"
	"time"
)

func GetMux(c *cron.Cron) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", getIndexHandler(c))
	mux.HandleFunc("POST /{$}", postIndexHandler(c))
	return mux
}

func getIndexHandler(c *cron.Cron) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		b.WriteString(`<style>html{background-color:#222;color:#eee;}</style>`)
		jobRuns := c.RunningJobs()
		b.WriteString(`
Running jobs<br />
<table>
	<thead>
		<th>Entry ID</th>
		<th>Run ID</th>
		<th>Started at</th>
		<th>Actions</th>
	</thead>
	<tbody>
`)
		for _, jobRun := range jobRuns {
			b.WriteString(`
	<tr>
		<td>` + string(jobRun.Entry.ID) + `</td>
		<td>` + jobRun.RunID + `</td>
		<td>` + jobRun.Entry.Label + `</td>
		<td>` + jobRun.StartedAt.Format(time.DateTime) + `</td>
		<td>
			<form method="POST" style="display: inline-block;">
				<input type="hidden" name="entryID" value="` + string(jobRun.Entry.ID) + `" />
				<input type="hidden" name="runID" value="` + jobRun.RunID + `" />
				<input type="submit" value="Cancel" />
			</form>
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

func postIndexHandler(c *cron.Cron) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entryID := cron.EntryID(r.PostFormValue("entryID"))
		runID := r.PostFormValue("runID")
		c.CancelRun(entryID, runID)
		w.Header().Set("Location", "/")
		w.WriteHeader(http.StatusFound)
	}
}
