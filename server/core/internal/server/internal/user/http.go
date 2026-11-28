// Package user provides HTTP handlers for user operations.
package user

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	userCore "github.com/oxynote/oxynote/server/core/internal/user"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
)

// Handler holds dependencies required for user-related operations.
type Handler struct {
	log       *slog.Logger
	db        DB
	storer    Storer
	publicURL string
}

// NewHandler creates a new handler instance with the provided logger and
// database. publicURL is the origin the Location of an image is built on.
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

// RetrieveUserImage handles the retrieval of a user's image.
func (h *Handler) RetrieveUserImage(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.RequireSession(h.log, w, r); !ok {
		return
	}

	userID, err := httpserver.ExtractParam(r, "userId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	obj, found, err := h.storer.Retrieve(r.Context(), userCore.ImageFolder, userID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !found {
		httpserver.RespondError(h.log, w, errutil.ErrNotFound)
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	file, err := httpserver.FormFile(w, r, "image", storage.ImagePolicy.MaxUploadBytes(), storage.ErrSizeLimitExceeded)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}
	defer file.Close() //nolint:errcheck // error provides no meaningful info

	data, contentType, err := storage.ReadObject(file, storage.ImagePolicy)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.storer.Upload(r.Context(), userCore.ImageFolder, session.UserID, data, contentType)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	imageLocation := httpserver.CacheBust(h.publicURL + userCore.ImagePath(session.UserID))

	err = h.db.UpdateUserImage(r.Context(), session.UserID, imageLocation)
	if err != nil {
		derr := h.storer.Delete(r.Context(), userCore.ImageFolder, session.UserID)
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

// FetchUserSpotlights handles the retrieval of the state of the session
// user's spotlights.
func (h *Handler) FetchUserSpotlights(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	spotlights, err := h.db.FetchUserSpotlights(r.Context(), session.UserID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, spotlights, http.StatusOK)
}

// UpdateUserSpotlights handles a change to the state of the session user's
// spotlights.
func (h *Handler) UpdateUserSpotlights(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	var inp userCore.SpotlightsInput

	if err := httpserver.DecodeJSON(r, &inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := inp.Validate(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.db.UpsertUserSpotlights(r.Context(), session.UserID, inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, nil, http.StatusNoContent)
}

// DB is an interface that combines sqlutil.DB and DBAgent.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	sqlutil.DB
	DBAgent
}

// DBAgent is an interface that handles communication with the users database.
type DBAgent interface {
	// UpdateUserImage should update the user's image URL.
	UpdateUserImage(ctx context.Context, userID, image string) error

	// FetchUserSpotlights should retrieve the state of the user's spotlights.
	FetchUserSpotlights(ctx context.Context, userID string) (userCore.Spotlights, error)

	// UpsertUserSpotlights should store the state of the user's spotlights,
	// keeping a dismissed spotlight dismissed.
	UpsertUserSpotlights(ctx context.Context, userID string, inp userCore.SpotlightsInput) error
}

// Storer is an interface that defines methods for uploading and retrieving objects.
//
//go:generate ../../../../scripts/codegen/mock -t internal Storer
type Storer interface {
	// Upload should store the object's bytes under the given content
	// type.
	Upload(ctx context.Context, folder, id string, data []byte, contentType string) error

	// Retrieve retrieves an object by its ID.
	Retrieve(ctx context.Context, folder, id string) (*storage.ObjectInfo, bool, error)

	// Delete deletes an object by its ID.
	Delete(ctx context.Context, folder, id string) error
}
