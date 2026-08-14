package dochandler

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/server/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// _documentFilesFolderFormat is the folder format for document files.
const _documentFilesFolderFormat = "organizations/%s/documents/%s/files"

// UploadDocumentFile handles the upload of a file to a document.
func (h *Handler) UploadDocumentFile(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// Verify document exists and belongs to the organization.
	err = h.db.CheckDocumentExists(r.Context(), documentID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	fileID := r.URL.Query().Get("id")
	if fileID == "" {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	location := document.Location(r.URL.Query().Get("location"))
	if !location.Valid() {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}
	defer file.Close() //nolint:errcheck // error provides no meaningful info

	folder := fmt.Sprintf(_documentFilesFolderFormat, session.ActiveOrganizationID, documentID)

	err = h.storer.Upload(r.Context(), folder, fileID, file)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.db.InsertDocumentFile(
		r.Context(),
		document.NewFile(
			fileID,
			location,
			documentID,
			session.ActiveOrganizationID,
		),
	)
	if err != nil {
		// Clean up the uploaded file if DB insert fails.
		if derr := h.storer.Delete(r.Context(), folder, fileID); derr != nil {
			h.log.Error("deleting file after DB failure", "error", derr.Error())
		}

		httpserver.RespondError(h.log, w, err)

		return
	}

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusOK,
		httpserver.LocationHeader(
			fmt.Sprintf(h.fileLocationFormat, documentID, fileID),
		),
	)
}

// RetrieveDocumentFile handles the retrieval of a file from a document.
func (h *Handler) RetrieveDocumentFile(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractIDParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// Verify file exists and belongs to the organization.
	_, err = h.db.FetchDocumentFile(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	folder := fmt.Sprintf(_documentFilesFolderFormat, session.ActiveOrganizationID, documentID)

	obj, found, err := h.storer.Retrieve(r.Context(), folder, id)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !found {
		http.NotFound(w, r)
		return
	}

	defer obj.Body.Close() //nolint:errcheck // error provides no meaningful info

	if match := r.Header.Get("If-None-Match"); match == obj.ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Content-Type", obj.ContentType)

	_, err = io.Copy(w, obj.Body)
	if err != nil {
		h.log.Error("streaming document file", "error", err.Error())
		return
	}
}

// extractIDParameter extracts the ID from the request parameters.
func (h *Handler) extractIDParameter(r *http.Request) (string, error) {
	return httpserver.ExtractParam(r, "id")
}

// FilesDBAgent is an interface that handles communication with the document files database.
type FilesDBAgent interface {
	// InsertDocumentFile should insert the document file.
	InsertDocumentFile(ctx context.Context, f document.File) error

	// FetchDocumentFile should fetch the document file for the given block id.
	FetchDocumentFile(ctx context.Context, blockID, organizationID string) (*document.File, error)
}

// Storer is an interface that defines methods for uploading and retrieving objects.
type Storer interface {
	// Upload uploads a new object.
	Upload(ctx context.Context, folder, id string, r io.Reader) error

	// Retrieve retrieves an object by its ID.
	Retrieve(ctx context.Context, folder, id string) (*storage.ObjectInfo, bool, error)

	// Delete deletes an object by its ID.
	Delete(ctx context.Context, folder, id string) error
}
