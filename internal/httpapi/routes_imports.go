package httpapi

import (
	"io"
	"net/http"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/imports"
	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/platform/apperr"
)

func (a *api) importTemplate(w http.ResponseWriter, r *http.Request) {
	if err := PrincipalFrom(r.Context()).Require("import.run"); err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="nithidesk-diary-template.csv"`)
	_, _ = w.Write(imports.Template())
}

func (a *api) listImports(w http.ResponseWriter, r *http.Request) {
	out, err := a.s.Imports.List(r.Context(), PrincipalFrom(r.Context()))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) uploadImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, imports.MaxFileBytes+64*1024)
	if err := r.ParseMultipartForm(imports.MaxFileBytes + 64*1024); err != nil {
		a.fail(w, r, apperr.Invalid(apperr.Field("file", "import.file_size")))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, header, err := r.FormFile("file")
	if err != nil {
		a.fail(w, r, apperr.Invalid(apperr.Field("file", "required")))
		return
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, imports.MaxFileBytes+1))
	if err != nil {
		a.fail(w, r, apperr.Malformed)
		return
	}
	out, err := a.s.Imports.Upload(r.Context(), PrincipalFrom(r.Context()), r.FormValue("kind"), header.Filename, content)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	writeJSON(w, http.StatusCreated, out)
}

func (a *api) getImport(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Imports.Get(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	a.ok(w, out)
}

func (a *api) importRows(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Imports.Rows(r.Context(), PrincipalFrom(r.Context()), id, r.URL.Query().Get("status"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.ok(w, map[string]any{"items": out})
}

func (a *api) validateImport(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Imports.Validate(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	a.ok(w, out)
}

func (a *api) commitImport(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	version := 0
	if err == nil {
		version, err = ifMatch(r)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Imports.Commit(r.Context(), PrincipalFrom(r.Context()), id, version)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	a.ok(w, out)
}

func (a *api) rollbackImport(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.s.Imports.Rollback(r.Context(), PrincipalFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	etag(w, out.Version)
	a.ok(w, out)
}
