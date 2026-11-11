package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/liamphmurphy/vesper/server/internal/api"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/deck"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// maxSlideThemes caps how many themes one person saves.
const maxSlideThemes = 100

func slideThemeErr(err error) error {
	var pe *pgconn.PgError
	switch {
	case errors.As(err, &pe) && pe.Code == "23505":
		return apperr.ConflictField("name", "You already have a theme with that name")
	case errors.Is(err, pgx.ErrNoRows):
		return apperr.ErrNotFound
	}
	return err
}

func slideThemeOut(t store.DeckTheme) (api.SlideTheme, error) {
	th, err := deck.SavedTheme(t.Name, t.Fonts, t.Colors)
	if err != nil {
		return api.SlideTheme{}, err
	}
	c := th.Colors
	return api.SlideTheme{
		Id: t.ID, Name: t.Name, UpdatedAt: t.UpdatedAt,
		Fonts:  api.SlideThemeFonts{Heading: api.SlideThemeFontsHeading(th.Fonts["heading"]), Body: api.SlideThemeFontsBody(th.Fonts["body"])},
		Colors: api.SlideThemeColors{Background: c["background"], Text: c["text"], Heading: c["heading"], Accent: c["accent"], Accent2: c["accent2"], Muted: c["muted"], Surface: c["surface"]},
	}, nil
}

// slideThemeIn validates a theme and encodes its fonts and colours for storage.
func slideThemeIn(name string, f api.SlideThemeFonts, c api.SlideThemeColors) (string, []byte, []byte, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, nil, apperr.Invalid("name", "give the theme a name")
	}
	if _, builtin := deck.Themes[strings.ToLower(name)]; builtin {
		return "", nil, nil, apperr.ConflictField("name", "%s is a built-in theme; pick another name", name)
	}
	t := deck.Theme{
		Name:   name,
		Fonts:  map[string]string{"heading": string(f.Heading), "body": string(f.Body)},
		Colors: map[string]string{"background": c.Background, "text": c.Text, "heading": c.Heading, "accent": c.Accent, "accent2": c.Accent2, "muted": c.Muted, "surface": c.Surface},
	}
	if err := t.Validate(); err != nil {
		var ve *apperr.ValidationError
		if errors.As(err, &ve) {
			err = apperr.Invalid("theme", "%s", ve.Msg)
		}
		return "", nil, nil, err
	}
	fonts, err := json.Marshal(t.Fonts)
	if err != nil {
		return "", nil, nil, err
	}
	colors, err := json.Marshal(t.Colors)
	return t.Name, fonts, colors, err
}

func (s *Server) ListSlideThemes(ctx context.Context, _ api.ListSlideThemesRequestObject) (api.ListSlideThemesResponseObject, error) {
	u, _, ws := me(ctx)
	rows, err := s.Q.ListDeckThemes(ctx, store.ListDeckThemesParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	out := api.ListSlideThemes200JSONResponse{Themes: make([]api.SlideTheme, 0, len(rows))}
	for _, r := range rows {
		t, err := slideThemeOut(r)
		if err != nil {
			return nil, err
		}
		out.Themes = append(out.Themes, t)
	}
	return out, nil
}

func (s *Server) CreateSlideTheme(ctx context.Context, req api.CreateSlideThemeRequestObject) (api.CreateSlideThemeResponseObject, error) {
	u, _, ws := me(ctx)
	name, fonts, colors, err := slideThemeIn(req.Body.Name, req.Body.Fonts, req.Body.Colors)
	if err != nil {
		return nil, err
	}
	n, err := s.Q.CountDeckThemes(ctx, store.CountDeckThemesParams{WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	if n >= maxSlideThemes {
		return nil, apperr.Invalid("name", "you can keep at most %d themes; delete one first", maxSlideThemes)
	}
	row, err := s.Q.CreateDeckTheme(ctx, store.CreateDeckThemeParams{WorkspaceID: ws, UserID: u.ID, Name: name, Fonts: fonts, Colors: colors})
	if err != nil {
		return nil, slideThemeErr(err)
	}
	out, err := slideThemeOut(row)
	return api.CreateSlideTheme201JSONResponse(out), err
}

func (s *Server) UpdateSlideTheme(ctx context.Context, req api.UpdateSlideThemeRequestObject) (api.UpdateSlideThemeResponseObject, error) {
	u, _, ws := me(ctx)
	cur, err := s.Q.GetDeckTheme(ctx, store.GetDeckThemeParams{ID: req.ThemeId, WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, slideThemeErr(err)
	}
	was, err := slideThemeOut(cur)
	if err != nil {
		return nil, err
	}
	name, f, c := was.Name, was.Fonts, was.Colors
	if req.Body.Name != nil {
		name = *req.Body.Name
	}
	if req.Body.Fonts != nil {
		f = *req.Body.Fonts
	}
	if req.Body.Colors != nil {
		c = *req.Body.Colors
	}
	name, fonts, colors, err := slideThemeIn(name, f, c)
	if err != nil {
		return nil, err
	}
	row, err := s.Q.UpdateDeckTheme(ctx, store.UpdateDeckThemeParams{ID: cur.ID, WorkspaceID: ws, UserID: u.ID, Name: name, Fonts: fonts, Colors: colors})
	if err != nil {
		return nil, slideThemeErr(err)
	}
	out, err := slideThemeOut(row)
	return api.UpdateSlideTheme200JSONResponse(out), err
}

func (s *Server) DeleteSlideTheme(ctx context.Context, req api.DeleteSlideThemeRequestObject) (api.DeleteSlideThemeResponseObject, error) {
	u, _, ws := me(ctx)
	n, err := s.Q.DeleteDeckTheme(ctx, store.DeleteDeckThemeParams{ID: req.ThemeId, WorkspaceID: ws, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, apperr.ErrNotFound
	}
	return api.DeleteSlideTheme204Response{}, nil
}
