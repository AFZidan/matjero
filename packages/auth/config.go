package auth

import "strings"

type Config struct {
	IssuerURL  string
	Audience   string
	RolesClaim string
}

func NormalizeIssuer(issuer string) string {
	return strings.TrimRight(strings.TrimSpace(issuer), "/")
}

func DefaultRolesClaim() string {
	return "urn:zitadel:iam:org:project:roles"
}
