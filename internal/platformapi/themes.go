package platformapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"dropshipping/internal/commerce"
	"dropshipping/internal/themes"
	"dropshipping/packages/httpx"
)

// ThemeDependencies wires the Theme Engine into the seller API. The Commerce
// service is used only to resolve the authenticated principal's seller identity;
// theme business logic lives in the themes package.
type ThemeDependencies struct {
	Themes   themes.Service
	Commerce commerce.Service
}

type themeServer struct {
	deps ThemeDependencies
}

// RegisterSellerThemeRoutes registers seller-scoped Theme management endpoints
// under /v1/seller. Every store-scoped operation enforces resource-level
// authorization: a seller may only manage themes for stores they own.
func RegisterSellerThemeRoutes(deps ThemeDependencies) func(r chi.Router) {
	s := themeServer{deps: deps}
	return func(r chi.Router) {
		r.Get("/seller/themes", s.handleListThemes)
		r.Get("/seller/themes/{key}/versions", s.handleListThemeVersions)
		r.Get("/seller/stores/{store_id}/theme", s.handleGetInstallation)
		r.Post("/seller/stores/{store_id}/theme/install", s.handleInstall)
		r.Get("/seller/stores/{store_id}/theme/draft", s.handleGetDraft)
		r.Put("/seller/stores/{store_id}/theme/draft", s.handleUpdateDraft)
		r.Post("/seller/stores/{store_id}/theme/publish", s.handlePublish)
		r.Post("/seller/stores/{store_id}/theme/discard", s.handleDiscardDraft)
		r.Post("/seller/stores/{store_id}/theme/upgrade", s.handleUpgrade)
		r.Post("/seller/stores/{store_id}/theme/preview", s.handleCreatePreview)
	}
}

func (s themeServer) sellerID(r *http.Request) (string, error) {
	subject, err := subjectFrom(r)
	if err != nil {
		return "", err
	}
	return s.deps.Commerce.ResolveSellerIDForSubject(r.Context(), subject)
}

func writeThemeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, themes.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "theme or store not found")
	case errors.Is(err, themes.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, "conflict", "theme version conflict")
	case errors.Is(err, themes.ErrSchemaMismatch):
		httpx.WriteError(w, http.StatusBadRequest, "schema_mismatch", "configuration does not match the theme schema")
	case errors.Is(err, themes.ErrUnsafeContent):
		httpx.WriteError(w, http.StatusBadRequest, "unsafe_content", "configuration contains prohibited executable content")
	case errors.Is(err, themes.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "invalid input")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

func (s themeServer) handleListThemes(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Themes.ListThemes(r.Context())
	if err != nil {
		writeThemeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ThemeCollectionResponse{Items: items})
}

func (s themeServer) handleListThemeVersions(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	theme, err := s.deps.Themes.GetThemeByKey(r.Context(), key)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	items, err := s.deps.Themes.ListThemeVersions(r.Context(), theme.ID)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ThemeVersionCollectionResponse{Items: items})
}

func (s themeServer) handleGetInstallation(w http.ResponseWriter, r *http.Request) {
	sellerID, err := s.sellerID(r)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	storeID := chi.URLParam(r, "store_id")
	inst, cfg, err := s.deps.Themes.GetInstallation(r.Context(), sellerID, storeID)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ThemeInstallationResponse{
		Installation:      inst,
		DraftConfig:       cfg.DraftConfig,
		PublishedConfig:   cfg.PublishedConfig,
		DraftRevision:     cfg.DraftRevision,
		PublishedRevision: cfg.PublishedRevision,
	})
}

func (s themeServer) handleInstall(w http.ResponseWriter, r *http.Request) {
	sellerID, err := s.sellerID(r)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	storeID := chi.URLParam(r, "store_id")
	var body ThemeInstallRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	inst, err := s.deps.Themes.Install(r.Context(), sellerID, storeID, body.ThemeKey, body.Version)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, ThemeInstallationResponse{Installation: inst})
}

func (s themeServer) handleGetDraft(w http.ResponseWriter, r *http.Request) {
	sellerID, err := s.sellerID(r)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	storeID := chi.URLParam(r, "store_id")
	config, rev, err := s.deps.Themes.GetDraftConfiguration(r.Context(), sellerID, storeID)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ThemeDraftResponse{Config: config, Revision: rev})
}

func (s themeServer) handleUpdateDraft(w http.ResponseWriter, r *http.Request) {
	sellerID, err := s.sellerID(r)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	storeID := chi.URLParam(r, "store_id")
	var body ThemeConfigRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	rev, err := s.deps.Themes.UpdateDraftConfiguration(r.Context(), sellerID, storeID, body.Config)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ThemeDraftResponse{Config: body.Config, Revision: rev})
}

func (s themeServer) handlePublish(w http.ResponseWriter, r *http.Request) {
	sellerID, err := s.sellerID(r)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	storeID := chi.URLParam(r, "store_id")
	rev, err := s.deps.Themes.PublishConfiguration(r.Context(), sellerID, storeID)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ThemePublishResponse{PublishedRevision: rev})
}

func (s themeServer) handleDiscardDraft(w http.ResponseWriter, r *http.Request) {
	sellerID, err := s.sellerID(r)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	storeID := chi.URLParam(r, "store_id")
	rev, err := s.deps.Themes.DiscardDraft(r.Context(), sellerID, storeID)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	config, _, err := s.deps.Themes.GetDraftConfiguration(r.Context(), sellerID, storeID)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ThemeDraftResponse{Config: config, Revision: rev})
}

func (s themeServer) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	sellerID, err := s.sellerID(r)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	storeID := chi.URLParam(r, "store_id")
	var body ThemeUpgradeRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.deps.Themes.UpgradeInstallation(r.Context(), sellerID, storeID, body.Version); err != nil {
		writeThemeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, StatusResponse{Status: "upgraded"})
}

func (s themeServer) handleCreatePreview(w http.ResponseWriter, r *http.Request) {
	sellerID, err := s.sellerID(r)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	storeID := chi.URLParam(r, "store_id")
	token, err := s.deps.Themes.CreatePreviewToken(r.Context(), sellerID, storeID)
	if err != nil {
		writeThemeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ThemePreviewResponse{Token: token})
}

// --- Theme API request/response types ---

type ThemeCollectionResponse = CollectionResponse[themes.Theme]

type ThemeVersionCollectionResponse = CollectionResponse[themes.ThemeVersion]

type ThemeInstallationResponse struct {
	Installation      themes.ThemeInstallation `json:"installation"`
	DraftConfig       map[string]any           `json:"draft_config,omitempty"`
	PublishedConfig   map[string]any           `json:"published_config,omitempty"`
	DraftRevision     int                      `json:"draft_revision"`
	PublishedRevision int                      `json:"published_revision"`
}

type ThemeDraftResponse struct {
	Config   map[string]any `json:"config"`
	Revision int            `json:"revision"`
}

type ThemePublishResponse struct {
	PublishedRevision int `json:"published_revision"`
}

type ThemePreviewResponse struct {
	Token string `json:"token"`
}

type ThemeConfigRequest struct {
	Config map[string]any `json:"config"`
}

type ThemeInstallRequest struct {
	ThemeKey string `json:"theme_key"`
	Version  string `json:"version,omitempty"`
}

type ThemeUpgradeRequest struct {
	Version string `json:"version"`
}
