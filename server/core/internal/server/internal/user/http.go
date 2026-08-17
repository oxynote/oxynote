// Package user provides HTTP handlers for user operations.
package user

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

// _userImageFolderFormat is the folder where user images are stored.
const _userImageFolderFormat = "organizations/%s/users/images"

// Handler holds dependencies required for user-related operations.
type Handler struct {
	log                 *slog.Logger
	db                  DB
	storer              Storer
	imageLocationFormat string
}

// NewHandler creates a new handler instance with the provided logger and database.
func NewHandler(
	log *slog.Logger,
	db DB,
	storer Storer,
	imageLocationFormat string,
) *Handler {
	return &Handler{
		log:                 log,
		db:                  db,
		storer:              storer,
		imageLocationFormat: imageLocationFormat,
	}
}

// RetrieveUserImage handles the retrieval of a user's image.
func (h *Handler) RetrieveUserImage(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	userID, err := httpserver.ExtractParam(r, "userId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	imageFolder := fmt.Sprintf(_userImageFolderFormat, session.ActiveOrganizationID)

	obj, found, err := h.storer.Retrieve(r.Context(), imageFolder, userID)
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

// UploadUserImage handles the upload of a user's image.
func (h *Handler) UploadUserImage(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}
	defer file.Close() //nolint:errcheck // error provides no meaningful info

	imageFolder := fmt.Sprintf(_userImageFolderFormat, session.ActiveOrganizationID)

	err = h.storer.Upload(r.Context(), imageFolder, session.UserID, file)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// Append a timestamp to the URL to prevent caching issues.
	imageLocation := fmt.Sprintf(h.imageLocationFormat, session.UserID) + "?v=" + timeutil.Now().Format("20060102150405")

	err = h.db.UpdateUserImage(r.Context(), session.UserID, imageLocation)
	if err != nil {
		derr := h.storer.Delete(r.Context(), imageFolder, session.UserID)
		if derr != nil {
			h.log.Error("deleting object after DB failure", slog.String("error", derr.Error()))
		}

		httpserver.RespondError(h.log, w, err)

		return
	}

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusCreated,
		httpserver.LocationHeader(imageLocation),
	)
}

// DB is an interface that combines sqlutil.DB and DBAgent.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	sqlutil.DB
	DBAgent
}

// Tx is an interface that combines sqlutil.Tx and DBAgent.
type Tx interface {
	sqlutil.Tx
	DBAgent
}

// DBAgent is an interface that handles communication with the users database.
type DBAgent interface {
	// UpdateUserImage should update the user's image URL.
	UpdateUserImage(ctx context.Context, userID, image string) error
}

// Storer is an interface that defines methods for uploading and retrieving objects.
//
//go:generate ../../../../scripts/codegen/mock -t internal Storer
type Storer interface {
	// Upload uploads a new object.
	Upload(ctx context.Context, folder, id string, r io.Reader) error

	// Retrieve retrieves an object by its ID.
	Retrieve(ctx context.Context, folder, id string) (*storage.ObjectInfo, bool, error)

	// Delete deletes an object by its ID.
	Delete(ctx context.Context, folder, id string) error
}
