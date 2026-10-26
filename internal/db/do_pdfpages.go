package db

import (
	"bytes"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"strconv"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

func registerPDFPages(mux *http.ServeMux, s *Store, pics *Pictures, tokens auth.Tokens) {
	mux.HandleFunc("POST /api/do/pdf-page", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), tokens)
		if !ok {
			return
		}
		if !s.Model().whole(env) {
			serve.Error(w, r, access.Forbidden("upload pdf pages in import mode"))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, fileLimit+1<<20)
		if err := r.ParseMultipartForm(formMemory); err != nil {
			serve.Error(w, r, access.Invalid("send the page as multipart form data: %v", err))
			return
		}
		defer r.MultipartForm.RemoveAll()
		page, err := strconv.Atoi(r.FormValue("page"))
		total, totalErr := strconv.Atoi(r.FormValue("pages"))
		if err != nil || totalErr != nil || page < 1 || total < page {
			serve.Error(w, r, access.Invalid("page and pages must name a page from 1 through the total"))
			return
		}
		id, original := r.FormValue("document"), r.FormValue("content")
		doc, found := s.Model().Table("DOCUMENT").Get(id)
		content, _ := s.Model().Table("CONTENT").Get(doc["content"])
		if !found || original == "" || doc["content"] != original || baseType(content["mime"]) != pdfType {
			serve.Error(w, r, access.Invalid("the document must still hold the pdf drawn"))
			return
		}
		file, _, err := r.FormFile("picture")
		if err != nil {
			serve.Error(w, r, access.Invalid("the picture is required"))
			return
		}
		defer file.Close()
		raw, err := io.ReadAll(file)
		if err != nil {
			serve.Error(w, r, access.Invalid("could not read the page"))
			return
		}
		if err := blob.Check(raw); err != nil {
			serve.Error(w, r, access.Invalid("the page: %v", err))
			return
		}
		if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
			serve.Error(w, r, access.Invalid("the page must be a png: %v", err))
			return
		}
		kept, err := keepBytes(r.Context(), s.Model(), pics.bucket, "image/png", raw)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		_, err = s.Transact(r.Context(), actor, func(tx *store.Tx[Model]) error {
			m := s.In(tx)
			if !m.whole(env) {
				return access.Forbidden("upload pdf pages in import mode")
			}
			now, found := m.Table("DOCUMENT").Get(id)
			if !found || now["content"] != original {
				return access.Invalid("the document no longer holds the pdf drawn")
			}
			count := strconv.Itoa(total)
			if now["page_count"] != "" && now["page_count"] != count {
				return access.Invalid("the pdf was previously reported to have %s pages", now["page_count"])
			}
			seen := map[string]bool{}
			existing := ""
			for _, child := range m.Table("DOCUMENT").Referencing("parent", id) {
				if child["relation"] != "page" {
					continue
				}
				seen[child["first_page"]] = true
				if child["first_page"] == strconv.Itoa(page) {
					existing = child["content"]
				}
			}
			if existing != "" {
				old, _ := m.Table("CONTENT").Get(existing)
				if old["hash"] != kept.hash {
					return access.Invalid("page %d already holds different pixels", page)
				}
				return nil
			}
			if page != len(seen)+1 {
				return access.Invalid("upload pdf pages in order; the next page is %d", len(seen)+1)
			}
			if now["rendered"] != "" {
				return access.Invalid("the pdf's pages are already complete")
			}
			stamp := time.Now().In(School).Format(cells.StampFormat)
			writes := &pendingWrites{}
			content, err := stageContent(m, writes, map[string]string{}, kept)
			if err != nil {
				return err
			}
			number := strconv.Itoa(page)
			writes.add(Edit{Insert: "DOCUMENT", Row: map[string]any{"parent": id, "relation": "page", "content": content, "first_page": number, "last_page": number, "name": spanName(page, page, total), "order": fmt.Sprintf("%020d1", page), "extracted": stamp}})
			set := map[string]any{"page_count": count}
			if page == total {
				set["rendered"] = stamp
			}
			writes.add(Edit{Set: id, Cells: set})
			_, err = stageEdits(s, tx, env, writes.edits, "pdf page", true)
			return err
		})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		serve.Write(w, r, http.StatusOK, stored{Result: []string{id}, Hash: blob.Hash(raw)})
	})
}
