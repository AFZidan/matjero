package openapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

type RouterConfig struct {
	Enabled   bool
	SpecPath  string
	DocsPath  string
	SpecBytes []byte
}

func NewRouter(cfg RouterConfig) chi.Router {
	r := chi.NewRouter()
	if !cfg.Enabled {
		return r
	}

	specPath := cfg.SpecPath
	if specPath == "" {
		specPath = "/openapi.json"
	}

	docsPath := cfg.DocsPath
	if docsPath == "" {
		docsPath = "/docs"
	}
	docsPath = strings.TrimRight(docsPath, "/")

	r.Get(specPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfg.SpecBytes)
	})

	swaggerHandler := httpSwagger.Handler(httpSwagger.URL(specPath))
	r.Get(docsPath, func(w http.ResponseWriter, req *http.Request) {
		cloned := req.Clone(req.Context())
		cloned.RequestURI = docsPath + "/index.html"
		cloned.URL.Path = docsPath + "/index.html"
		swaggerHandler.ServeHTTP(w, cloned)
	})
	r.Handle(docsPath+"/*", swaggerHandler)

	return r
}
