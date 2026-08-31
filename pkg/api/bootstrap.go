package api

import (
	"github.com/matjeroapps/core/packages/auth"
	"github.com/matjeroapps/core/packages/i18n"
	"github.com/matjeroapps/core/pkg/markets"
)

type Bootstrap struct {
	App              string           `json:"app"`
	Actor            string           `json:"actor"`
	Locale           i18n.Locale      `json:"locale"`
	Direction        string           `json:"direction"`
	SupportedLocales []i18n.Locale    `json:"supported_locales"`
	Principal        *auth.Principal  `json:"principal,omitempty"`
	Markets          []markets.Market `json:"markets"`
}

func NewBootstrap(app, actor string, principal *auth.Principal, locale i18n.Locale, supported []i18n.Locale, marketsList []markets.Market) Bootstrap {
	return Bootstrap{
		App:              app,
		Actor:            actor,
		Locale:           locale,
		Direction:        i18n.Direction(locale),
		SupportedLocales: append([]i18n.Locale(nil), supported...),
		Principal:        principal,
		Markets:          append([]markets.Market(nil), marketsList...),
	}
}
