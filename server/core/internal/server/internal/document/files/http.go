// Package files provides HTTP handlers for document file storage.
package files

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/oxynote/oxynote/server/core/internal/document/file"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// _fileIDPattern matches the block-uid shapes clients mint for file ids
// (21-char nanoids or hyphenated UUIDs). The charset shuts out the path
// separators and dot segments that would otherwise let an id escape its
// storage folder once joined into an object key.
var _fileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Handler holds dependencies required for document file operations.
type Handler struct {
	log                *slog.Logger
	db                 DB
	storer             Storer
	fileLocationFormat string
}

// NewHandler creates a new handler instance with the provided logger and database.
func NewHandler(
	log *slog.Logger,
	db DB,
	storer Storer,
	fileLocationFormat string,
) *Handler {
	return &Handler{
		log:                log,
		db:                 db,
		storer:             storer,
		fileLocationFormat: fileLocationFormat,
	}
}

// UploadDocumentFile handles the upload of a file to a document.
func (h *Handler) UploadDocumentFile(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
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
	if !_fileIDPattern.MatchString(fileID) {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	location := file.Location(r.URL.Query().Get("location"))
	if !location.Valid() {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	body, err := httpserver.FormFile(w, r, "file", storage.MaxUploadBytes, storage.ErrSizeLimitExceeded)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}
	defer body.Close() //nolint:errcheck // error provides no meaningful info

	folder := file.Folder(session.ActiveOrganizationID, documentID)

	// the row is written before the object so that a crash can only ever
	// leave a row without an object, which the file manager reaps. An
	// object without a row would be invisible to every cleanup path.
	err = h.db.InsertDocumentFile(
		r.Context(),
		file.NewFile(
			fileID,
			location,
			file.Key(session.ActiveOrganizationID, documentID, fileID),
			documentID,
			session.ActiveOrganizationID,
		),
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.storer.Upload(r.Context(), folder, fileID, body)
	if err != nil {
		if derr := h.db.DeleteDocumentFile(r.Context(), fileID); derr != nil {
			h.log.Error("deleting file row after upload failure", "error", derr.Error())
		}

		httpserver.RespondError(h.log, w, err)

		return
	}

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusCreated,
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

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := httpserver.ExtractParam(r, "id")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// Verify file exists and belongs to the organization.
	f, err := h.db.FetchDocumentFile(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// the object key is built from the document in the url, so a file
	// belonging to another document would merely miss the key; reject it
	// as the authorization failure it is instead.
	if f.DocumentID.V != documentID {
		httpserver.RespondError(h.log, w, errutil.ErrNotFound)
		return
	}

	folder := file.Folder(session.ActiveOrganizationID, documentID)

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

	httpserver.ServeObject(
		h.log,
		w,
		r,
		obj.ETag,
		obj.ContentType,
		obj.Body,
	)
}

// DB is an interface that handles communication with the document files database.
//
//go:generate ../../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	DBAgent

	// CheckDocumentExists returns nil if the document exists and belongs to the given organization.
	CheckDocumentExists(ctx context.Context, id xid.ID, organizationID string) error
}

// DBAgent is an interface that handles communication with the document files database.
type DBAgent interface {
	// InsertDocumentFile should insert the document file.
	InsertDocumentFile(ctx context.Context, f file.File) error

	// FetchDocumentFile should fetch the document file for the given block id.
	FetchDocumentFile(ctx context.Context, blockID, organizationID string) (*file.File, error)

	// DeleteDocumentFile should remove the document file row.
	DeleteDocumentFile(ctx context.Context, id string) error
}

// Storer is an interface that defines methods for uploading and retrieving objects.
//
//go:generate ../../../../../scripts/codegen/mock -t internal Storer
type Storer interface {
	// Upload uploads a new object.
	Upload(ctx context.Context, folder, id string, r io.Reader) error

	// Retrieve retrieves an object by its ID.
	Retrieve(ctx context.Context, folder, id string) (*storage.ObjectInfo, bool, error)
}
