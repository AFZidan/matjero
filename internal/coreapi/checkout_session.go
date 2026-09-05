package coreapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/matjeroapps/core/packages/httpx"
	"github.com/matjeroapps/core/pkg/commerce"
)

func checkoutSessionResponse(session commerce.CheckoutSession, rawCapability string) CheckoutSessionResponse {
	return CheckoutSessionResponse{
		ID: session.ID, CartID: session.CartID, Status: session.Status,
		ExpiresAt: session.ExpiresAt, CustomerID: session.CustomerID,
		GuestOrderAccessToken: rawCapability,
	}
}

func (s *server) handleCreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.scopeFor(w, r)
	if !ok {
		return
	}
	session, rawCapability, err := s.deps.Repo.CreateCheckoutSession(
		r.Context(), scope.StoreID(), cartTokenFromRequest(r), nil, s.deps.Commerce.CheckoutSessionLifetime,
	)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, checkoutSessionResponse(session, rawCapability))
}

func (s *server) handleEvaluateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.scopeFor(w, r)
	if !ok {
		return
	}
	var body CheckoutFinalizeRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	request := commerce.FinalizeRequest{
		SessionID: rPathSessionID(r),
		ShippingAddress: commerce.ShippingAddress{
			RecipientName: body.ShippingAddress.RecipientName,
			AddressLine1:  body.ShippingAddress.AddressLine1,
			AddressLine2:  body.ShippingAddress.AddressLine2,
			City:          body.ShippingAddress.City,
			Region:        body.ShippingAddress.Region,
			PostalCode:    body.ShippingAddress.PostalCode,
			CountryCode:   body.ShippingAddress.CountryCode,
		},
		ContactEmail: body.ContactEmail,
	}
	decision, err := s.deps.Repo.EvaluateCheckoutSession(r.Context(), scope.StoreID(), request)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, CheckoutDecisionResponse{SessionID: decision.SessionID, Status: decision.Status, Replay: decision.Replay})
}

func rPathSessionID(r *http.Request) string {
	return chi.URLParam(r, "sessionID")
}
