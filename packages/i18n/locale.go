package i18n

import (
	"context"
	"net/http"
	"strings"

	"golang.org/x/text/language"
)

type Locale string

const (
	LocaleArabic  Locale = "ar"
	LocaleEnglish Locale = "en"
)

var SupportedLocales = []Locale{LocaleArabic, LocaleEnglish}

var matcher = language.NewMatcher([]language.Tag{
	language.Arabic,
	language.English,
})

type contextKey string

const localeKey contextKey = "locale"

func Default() Locale {
	return LocaleEnglish
}

func Direction(locale Locale) string {
	if locale == LocaleArabic {
		return "rtl"
	}
	return "ltr"
}

func Negotiate(header string) Locale {
	if strings.TrimSpace(header) == "" {
		return Default()
	}

	tags, _, err := language.ParseAcceptLanguage(header)
	if err != nil || len(tags) == 0 {
		return Default()
	}

	tag, _, confidence := matcher.Match(tags...)
	if confidence == language.No {
		return Default()
	}

	if base, _ := tag.Base(); base.String() == "ar" {
		return LocaleArabic
	}
	return LocaleEnglish
}

func FromContext(ctx context.Context) Locale {
	locale, _ := ctx.Value(localeKey).(Locale)
	if locale == "" {
		return Default()
	}
	return locale
}

func Middleware(defaultLocale Locale) func(http.Handler) http.Handler {
	if defaultLocale == "" {
		defaultLocale = Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			locale := negotiateRequestLocale(r, defaultLocale)
			ctx := context.WithValue(r.Context(), localeKey, locale)
			w.Header().Set("Content-Language", string(locale))
			w.Header().Set("X-Locale", string(locale))
			w.Header().Set("X-Direction", Direction(locale))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func negotiateRequestLocale(r *http.Request, defaultLocale Locale) Locale {
	if queryLocale := Locale(r.URL.Query().Get("locale")); queryLocale == LocaleArabic || queryLocale == LocaleEnglish {
		return queryLocale
	}
	if headerLocale := Negotiate(r.Header.Get("Accept-Language")); headerLocale != "" {
		return headerLocale
	}
	return defaultLocale
}
