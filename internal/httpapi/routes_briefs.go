package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/briefs"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/clock"
)

// ---------------------------------------------------------------------------
// Daily brief (doc 06 §6.3, ADR 0007)
// ---------------------------------------------------------------------------

func (a *api) dailyBrief(w http.ResponseWriter, r *http.Request) {
	date, err := dateParam(r, "date", clock.Today())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Briefs.Daily(r.Context(), PrincipalFrom(r.Context()), date, intParam(r, "days", 1),
		r.URL.Query().Get("scope"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}

// requestBriefPDF queues a render; the client polls briefRender until the
// status is ready, then fetches the file.
func (a *api) requestBriefPDF(w http.ResponseWriter, r *http.Request) {
	var in briefs.RenderInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Briefs.RequestRender(r.Context(), PrincipalFrom(r.Context()), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (a *api) briefRender(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Briefs.GetRender(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, out)
}

// briefFile streams the PDF through the API rather than redirecting to a
// signed storage URL: the brief lists clients and instructions, so it gets
// the "highly restricted" treatment of ADR 0005. The file name carries no
// client or matter name.
func (a *api) briefFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	body, size, err := a.s.Briefs.OpenFile(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()
	h := w.Header()
	h.Set("Content-Type", "application/pdf")
	h.Set("Content-Length", strconv.FormatInt(size, 10))
	h.Set("Content-Disposition", `attachment; filename="nithidesk-brief-`+id.String()+`.pdf"`)
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		// Headers are sent; the client sees a truncated download. Log the
		// identifier only.
		a.logger.Warn("brief download interrupted", slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("object_id", id.String()), slog.String("error", err.Error()))
	}
}
