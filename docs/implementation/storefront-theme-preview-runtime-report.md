# Storefront Theme Preview Runtime Report

## Contract

Core now supports draft Theme preview consumption on `GET /internal/v1/storefront/store`
through the internal `X-Matjero-Storefront-Preview` header.

The header carries the signed preview token issued by the Theme service. It is
used only between the seller-owned storefront API and Core. It is not browser
authentication, and it does not replace the normal service-token `Authorization`
header.

Normal storefront bootstrap requests still return the published Theme. Valid
preview bootstrap requests return the same store, market, currency, locale and
public settings as normal bootstrap, but replace `StoreBootstrap.Theme` with the
current draft Theme configuration.

## Security Binding

Preview resolution keeps the existing tenant boundary:

1. service authentication runs first;
2. Core reads `X-Matjero-Storefront-Host`;
3. the Store is resolved from that trusted host;
4. the preview token is verified;
5. the token claims must match the resolved Store, current active Theme
   Installation, and exact current draft revision.

Core never uses the token's Store ID to choose a Store. Tokens for another Store,
old installations, stale draft revisions, malformed tokens, bad signatures,
expired tokens and missing installations all fail closed with the generic
storefront-unavailable outcome. Missing `THEME_PREVIEW_SECRET` remains the
separate `preview_unavailable` failure.

The draft configuration is revalidated against the current Theme Version schema
and checked for unsafe executable content before it is rendered.

## Cache Behavior

Preview responses are not public-cacheable:

- `X-Matjero-Storefront-Revision` is omitted;
- `Cache-Control: private, no-store` is set;
- preview reads do not bump the storefront revision.

Normal storefront bootstrap is unchanged and still carries the revision header.
Draft edits remain invisible to public cache generation until publish.

## Errors

Preview-specific invalid state deliberately avoids becoming a token validity
oracle. Invalid, stale and cross-store tokens collapse to the existing
`storefront_unavailable` error. Missing preview secret continues to return the
existing `preview_unavailable` error.

## Tests

Coverage was added for:

- valid preview token resolution;
- tampered, malformed and expired tokens;
- wrong Store, wrong installation, missing installation and stale draft revision;
- Theme switch invalidating an old token;
- published versus draft bootstrap through the Core API;
- prevention of draft leakage to a following normal bootstrap;
- preview cache headers and absence of the revision header;
- cross-store token rejection;
- publish making the draft public through the normal bootstrap path.

## Limitations

This is the Core half only. Seller Stage B still needs to forward the browser
preview token to Core as `X-Matjero-Storefront-Preview` from `storefront-api`,
skip its response cache when preview is present, and construct the seller-facing
preview URL.

No migration, Redis behavior, RabbitMQ behavior, or GoThrottle integration was
added.
