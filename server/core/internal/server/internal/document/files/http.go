// Package files provides HTTP handlers for document file storage.
package files

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/file"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

// _fileIDLength is the length of a block uid: every editor mints
// 21-character nanoids, and the retrieval route relies on it to cut the
// id off the front of "<id>-<file name>".
const _fileIDLength = 21

// _fileIDPattern matches a block uid. The charset shuts out the path
// separators and dot segments that would otherwise let an id escape its
// storage folder once joined into an object key.
var _fileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{21}$`)

// _maxFileNameLength caps the name recorded for an upload, in runes.
const _maxFileNameLength = 255

// _fallbackFileName names an upload whose multipart part carried no
// usable name.
const _fallbackFileName = "file"

// Handler holds dependencies required for document file operations.
type Handler struct {
	log       *slog.Logger
	db        DB
	storer    Storer
	publicURL string
}

// NewHandler creates a new handler instance with the provided logger and
// database. publicURL is the origin the Location of an upload is built on.
func NewHandler(
	log *slog.Logger,
	db DB,
	storer Storer,
	publicURL string,
) *Handler {
	return &Handler{
		log:       log,
		db:        db,
		storer:    storer,
		publicURL: publicURL,
	}
}

// UploadDocumentFile handles the upload of a file to a document.
func (h *Handler) UploadDocumentFile(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
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

	policy, ok := uploadPolicy(r.URL.Query().Get("kind"))
	if !ok {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	body, err := httpserver.FormFile(w, r, "file", policy.MaxUploadBytes(), storage.ErrSizeLimitExceeded)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}
	defer body.Close() //nolint:errcheck // error provides no meaningful info

	// the object is read and checked before the row is written, so a
	// rejected upload never leaves a row behind.
	data, contentType, err := storage.ReadObject(body, policy)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	name := fileName(r.MultipartForm.File["file"][0].Filename)

	f := file.NewFile(
		fileID,
		location,
		file.Key(session.ActiveOrganizationID, documentID, fileID),
		documentID,
		session.ActiveOrganizationID,
		name,
		int64(len(data)),
		contentType,
	)

	folder, key := path.Split(f.StorageKey)

	// the row is written before the object so that a crash can only ever
	// leave a row without an object, which the file manager reaps. An
	// object without a row would be invisible to every cleanup path.
	err = h.db.InsertDocumentFile(r.Context(), f)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.storer.Upload(r.Context(), folder, key, data, contentType)
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
		struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Size        int64  `json:"size"`
			ContentType string `json:"contentType"`
		}{
			ID:          f.ID,
			Name:        f.Name,
			Size:        f.Size,
			ContentType: f.ContentType,
		},
		http.StatusCreated,
		httpserver.LocationHeader(
			h.publicURL+document.FilePath(documentID, fileID, f.Name),
		),
	)
}

// fileID cuts the id off a "<id>-<file name>" route segment, reporting
// whether the segment has that shape.
func fileID(ref string) (string, bool) {
	if len(ref) <= _fileIDLength || ref[_fileIDLength] != '-' {
		return "", false
	}

	id := ref[:_fileIDLength]
	if !_fileIDPattern.MatchString(id) {
		return "", false
	}

	return id, true
}

// uploadPolicy maps the kind of block an upload belongs to onto the
// storage policy it is admitted under, reporting whether the kind is one
// at all.
func uploadPolicy(kind string) (storage.Policy, bool) {
	switch kind {
	case "image":
		return storage.ImagePolicy, true
	case "file":
		return storage.FilePolicy, true
	default:
		return storage.Policy{}, false
	}
}

// fileName reduces the name a multipart part carried to a bare file name:
// a browser may send a path, Windows ones with backslashes, and the part
// may carry no name at all.
func fileName(raw string) string {
	name := path.Base(strings.ReplaceAll(raw, "\\", "/"))
	name = strings.TrimSpace(name)

	if name == "." || name == "/" {
		name = ""
	}

	if utf8.RuneCountInString(name) > _maxFileNameLength {
		name = string([]rune(name)[:_maxFileNameLength])
	}

	if name == "" {
		return _fallbackFileName
	}

	return name
}

// RetrieveDocumentFile handles the retrieval of a file from a document.
// The route's last segment is "<id>-<file name>": only the id identifies
// the file, the name makes the address read as the file it is.
func (h *Handler) RetrieveDocumentFile(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	documentID, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ref, err := httpserver.ExtractParam(r, "ref")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, ok := fileID(ref)
	if !ok {
		httpserver.RespondError(h.log, w, errutil.ErrNotFound)
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

	folder, key := path.Split(f.StorageKey)

	obj, found, err := h.storer.Retrieve(r.Context(), folder, key)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !found {
		http.NotFound(w, r)
		return
	}

	defer obj.Body.Close() //nolint:errcheck // error provides no meaningful info

	// the row's disposition decides whether a browser renders the file,
	// and nosniff stops it second-guessing the type the row recorded.
	w.Header().Set("Content-Disposition", f.Disposition())
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// the row carries the type detected at upload; the store's own value
	// is a re-sniff on the file system backend, which cannot tell a
	// document from a zip.
	httpserver.ServeObject(
		h.log,
		w,
		r,
		obj.ETag,
		f.ContentType,
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
	// Upload should store the object's bytes under the given content
	// type.
	Upload(ctx context.Context, folder, id string, data []byte, contentType string) error

	// Retrieve retrieves an object by its ID.
	Retrieve(ctx context.Context, folder, id string) (*storage.ObjectInfo, bool, error)
}
