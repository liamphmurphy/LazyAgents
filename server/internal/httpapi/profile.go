package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
	"github.com/liamphmurphy/lazyagents/server/internal/blob"
	"github.com/liamphmurphy/lazyagents/server/internal/personas"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

// A person's profile: display name, role title and picture, editable only by
// that person. Pictures follow the persona avatar rules and live in the blob
// bucket under users/<workspace>/<user>/avatar/<uuid>.

const maxProfileField = 80

// userAvatarURL busts the browser cache whenever the profile changes.
func userAvatarURL(u store.User) *string {
	if u.AvatarKey == nil {
		return nil
	}
	v := fmt.Sprintf("/api/v1/users/%s/avatar?v=%d", u.ID, u.UpdatedAt.Unix())
	return &v
}

// initialsOf mirrors the web's initialsOf: the first letter of the first and
// last words, upper-cased.
func initialsOf(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}
	first, _ := utf8.DecodeRuneInString(parts[0])
	out := string(first)
	if len(parts) > 1 {
		last, _ := utf8.DecodeRuneInString(parts[len(parts)-1])
		out += string(last)
	}
	return strings.ToUpper(out)
}

// profileField trims v and checks it is at most maxProfileField runes with no
// control characters.
func profileField(field, v string, required bool) (string, error) {
	v = strings.TrimSpace(v)
	if required && v == "" {
		return "", apperr.Invalid(field, "required")
	}
	if utf8.RuneCountInString(v) > maxProfileField {
		return "", apperr.Invalid(field, "at most %d characters", maxProfileField)
	}
	if strings.IndexFunc(v, unicode.IsControl) >= 0 {
		return "", apperr.Invalid(field, "must not contain control characters")
	}
	return v, nil
}

func (s *Server) GetProfile(ctx context.Context, _ api.GetProfileRequestObject) (api.GetProfileResponseObject, error) {
	u, _, _ := me(ctx)
	return api.GetProfile200JSONResponse(toUser(u)), nil
}

func (s *Server) UpdateProfile(ctx context.Context, req api.UpdateProfileRequestObject) (api.UpdateProfileResponseObject, error) {
	u, _, ws := me(ctx)
	if req.Body == nil || (req.Body.Name == nil && req.Body.RoleTitle == nil) {
		return nil, apperr.Invalid("body", "send name or role_title")
	}
	name, role := u.Name, u.RoleTitle
	var err error
	if req.Body.Name != nil {
		if name, err = profileField("name", *req.Body.Name, true); err != nil {
			return nil, err
		}
	}
	if req.Body.RoleTitle != nil {
		if role, err = profileField("role_title", *req.Body.RoleTitle, false); err != nil {
			return nil, err
		}
	}
	out, err := s.Q.UpdateUserProfile(ctx, store.UpdateUserProfileParams{
		ID: u.ID, WorkspaceID: ws, Name: name, Initials: initialsOf(name), RoleTitle: role,
	})
	if err != nil {
		return nil, err
	}
	return api.UpdateProfile200JSONResponse(toUser(out)), nil
}

// setAvatar points the caller's row at key (or clears it) and returns the key
// it replaced, which the caller deletes once this has committed. The row lock
// gives each replaced key to exactly one caller.
func (s *Server) setUserAvatar(ctx context.Context, u store.User, key, contentType *string) (store.User, *string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return u, nil, err
	}
	defer tx.Rollback(ctx)
	q := s.Q.WithTx(tx)
	cur, err := q.GetUserForUpdate(ctx, store.GetUserForUpdateParams{ID: u.ID, WorkspaceID: u.WorkspaceID})
	if err != nil {
		return u, nil, err
	}
	if key == nil && cur.AvatarKey == nil {
		return cur, nil, nil
	}
	out, err := q.SetUserAvatar(ctx, store.SetUserAvatarParams{ID: u.ID, WorkspaceID: u.WorkspaceID, AvatarKey: key, AvatarContentType: contentType})
	if err != nil {
		return u, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return u, nil, err
	}
	return out, cur.AvatarKey, nil
}

func (s *Server) UploadProfileAvatar(ctx context.Context, req api.UploadProfileAvatarRequestObject) (api.UploadProfileAvatarResponseObject, error) {
	u, _, ws := me(ctx)
	var ct string
	b, err := readAvatar(req.Body)
	if err == nil {
		ct, err = personas.CheckAvatar(b)
	}
	if err != nil {
		if body, status, ok := avatarRefusal(err); ok {
			if status == 413 {
				return api.UploadProfileAvatar413JSONResponse{ErrorJSONResponse: api.ErrorJSONResponse(body)}, nil
			}
			return api.UploadProfileAvatar415JSONResponse(body), nil
		}
		return nil, err
	}
	if s.Blobs == nil {
		return nil, errNoBlobs
	}
	key := "users/" + ws.String() + "/" + u.ID.String() + "/avatar/" + uuid.NewString()
	if err := s.Blobs.Put(ctx, s.ArtifactBucket, key, bytes.NewReader(b), int64(len(b)), ct); err != nil {
		return nil, err
	}
	out, old, err := s.setUserAvatar(ctx, u, &key, &ct)
	if err != nil {
		s.deleteAvatarBlob(ctx, &key)
		return nil, err
	}
	s.deleteAvatarBlob(ctx, old)
	return api.UploadProfileAvatar200JSONResponse(toUser(out)), nil
}

func (s *Server) DeleteProfileAvatar(ctx context.Context, _ api.DeleteProfileAvatarRequestObject) (api.DeleteProfileAvatarResponseObject, error) {
	u, _, _ := me(ctx)
	out, old, err := s.setUserAvatar(ctx, u, nil, nil)
	if err != nil {
		return nil, err
	}
	s.deleteAvatarBlob(ctx, old)
	return api.DeleteProfileAvatar200JSONResponse(toUser(out)), nil
}

func (s *Server) GetUserAvatar(ctx context.Context, req api.GetUserAvatarRequestObject) (api.GetUserAvatarResponseObject, error) {
	_, _, ws := me(ctx)
	u, err := s.Q.GetUser(ctx, store.GetUserParams{ID: req.UserId, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if u.AvatarKey == nil || u.AvatarContentType == nil || !slices.Contains(personas.AvatarTypes, *u.AvatarContentType) {
		return nil, apperr.ErrNotFound
	}
	if s.Blobs == nil {
		return nil, errNoBlobs
	}
	obj, err := s.Blobs.Get(ctx, s.ArtifactBucket, *u.AvatarKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, apperr.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return avatarResponse{body: obj.Body, contentType: *u.AvatarContentType, length: obj.ContentLength}, nil
}
