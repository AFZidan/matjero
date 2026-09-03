package commerce

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ListStoreDomains returns all domains for a specific store ordered with primary domain first.
func (r Repository) ListStoreDomains(ctx context.Context, storeID string) ([]StoreDomain, error) {
	if storeID == "" {
		return nil, ErrInvalidInput
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, store_id, domain, is_primary, verified_at, status, domain_type, verification_token, last_checked_at, created_at, updated_at
		FROM store_domains
		WHERE store_id = $1
		ORDER BY is_primary DESC, created_at ASC, id ASC
	`, storeID)
	if err != nil {
		return nil, fmt.Errorf("list store domains: %w", err)
	}
	defer rows.Close()

	return scanStoreDomains(rows)
}

// GetStoreDomain fetches a single store_domains row by ID.
func (r Repository) GetStoreDomain(ctx context.Context, domainID string) (StoreDomain, error) {
	if domainID == "" {
		return StoreDomain{}, ErrInvalidInput
	}
	var d StoreDomain
	err := r.pool.QueryRow(ctx, `
		SELECT id, store_id, domain, is_primary, verified_at, status, domain_type, verification_token, last_checked_at, created_at, updated_at
		FROM store_domains
		WHERE id = $1
	`, domainID).Scan(
		&d.ID, &d.StoreID, &d.Domain, &d.IsPrimary, &d.VerifiedAt, &d.Status, &d.DomainType, &d.VerificationToken, &d.LastCheckedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return StoreDomain{}, translatePGError(err, "get store domain")
	}
	return d, nil
}

// UpdateDomainVerificationStatus updates a domain's status, verified_at, and last_checked_at.
func (r Repository) UpdateDomainVerificationStatus(ctx context.Context, domainID, status string, verifiedAt, lastCheckedAt *time.Time) (StoreDomain, error) {
	if domainID == "" || status == "" {
		return StoreDomain{}, ErrInvalidInput
	}
	var d StoreDomain
	err := r.pool.QueryRow(ctx, `
		UPDATE store_domains
		SET status = $2,
		    verified_at = COALESCE($3, verified_at),
		    last_checked_at = $4,
		    updated_at = now()
		WHERE id = $1
		RETURNING id, store_id, domain, is_primary, verified_at, status, domain_type, verification_token, last_checked_at, created_at, updated_at
	`, domainID, status, verifiedAt, lastCheckedAt).Scan(
		&d.ID, &d.StoreID, &d.Domain, &d.IsPrimary, &d.VerifiedAt, &d.Status, &d.DomainType, &d.VerificationToken, &d.LastCheckedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return StoreDomain{}, translatePGError(err, "update domain verification status")
	}
	return d, nil
}

// ActivateCustomDomainTx demotes any existing primary domain for the store and promotes
// the target verified custom domain to active primary within a single transaction.
func (r Repository) ActivateCustomDomainTx(ctx context.Context, storeID, domainID string) (StoreDomain, error) {
	if storeID == "" || domainID == "" {
		return StoreDomain{}, ErrInvalidInput
	}

	var activated StoreDomain
	err := r.withTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var target StoreDomain
		err := tx.QueryRow(ctx, `
			SELECT id, store_id, domain, is_primary, verified_at, status, domain_type, verification_token, last_checked_at, created_at, updated_at
			FROM store_domains
			WHERE id = $1 AND store_id = $2
			FOR UPDATE
		`, domainID, storeID).Scan(
			&target.ID, &target.StoreID, &target.Domain, &target.IsPrimary, &target.VerifiedAt, &target.Status, &target.DomainType, &target.VerificationToken, &target.LastCheckedAt, &target.CreatedAt, &target.UpdatedAt,
		)
		if err != nil {
			return translatePGError(err, "get domain for activation")
		}

		if target.DomainType != "custom" {
			return fmt.Errorf("%w: platform domain cannot be activated via custom flow", ErrInvalidInput)
		}
		if target.Status != "verified" {
			return fmt.Errorf("%w: domain must be verified to activate (current status: %s)", ErrConflict, target.Status)
		}

		// Demote existing primary domain for this store.
		if _, err := tx.Exec(ctx, `
			UPDATE store_domains
			SET is_primary = false, updated_at = now()
			WHERE store_id = $1 AND is_primary = true
		`, storeID); err != nil {
			return translatePGError(err, "demote current primary domain")
		}

		// Promote target custom domain to active primary.
		err = tx.QueryRow(ctx, `
			UPDATE store_domains
			SET status = 'active',
			    is_primary = true,
			    updated_at = now()
			WHERE id = $1
			RETURNING id, store_id, domain, is_primary, verified_at, status, domain_type, verification_token, last_checked_at, created_at, updated_at
		`, domainID).Scan(
			&activated.ID, &activated.StoreID, &activated.Domain, &activated.IsPrimary, &activated.VerifiedAt, &activated.Status, &activated.DomainType, &activated.VerificationToken, &activated.LastCheckedAt, &activated.CreatedAt, &activated.UpdatedAt,
		)
		if err != nil {
			return translatePGError(err, "activate target domain")
		}
		return nil
	})

	return activated, err
}

// ListDomainsAdmin lists store domains across stores with optional filters and pagination.
func (r Repository) ListDomainsAdmin(ctx context.Context, filter AdminDomainFilter) ([]StoreDomain, error) {
	page := normalizePage(filter.Page)
	args := []any{}
	where := []string{"1=1"}

	if filter.StoreID != "" {
		args = append(args, filter.StoreID)
		where = append(where, fmt.Sprintf("d.store_id = $%d", len(args)))
	}
	if filter.SellerID != "" {
		args = append(args, filter.SellerID)
		where = append(where, fmt.Sprintf("s.seller_id = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("d.status = $%d", len(args)))
	}
	if filter.DomainType != "" {
		args = append(args, filter.DomainType)
		where = append(where, fmt.Sprintf("d.domain_type = $%d", len(args)))
	}
	if filter.Search != "" {
		args = append(args, "%"+strings.ToLower(filter.Search)+"%")
		where = append(where, fmt.Sprintf("LOWER(d.domain) LIKE $%d", len(args)))
	}

	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	args = append(args, page.Limit, page.Offset)

	query := fmt.Sprintf(`
		SELECT d.id, d.store_id, d.domain, d.is_primary, d.verified_at, d.status, d.domain_type, d.verification_token, d.last_checked_at, d.created_at, d.updated_at
		FROM store_domains d
		JOIN stores s ON s.id = d.store_id
		WHERE %s
		ORDER BY d.created_at DESC, d.id DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(where, " AND "), limitIdx, offsetIdx)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list admin domains: %w", err)
	}
	defer rows.Close()

	return scanStoreDomains(rows)
}

// DisableDomainTx disables a domain. If the disabled domain was the primary domain of the store,
// it attempts to promote an eligible active platform domain for the same store to primary.
func (r Repository) DisableDomainTx(ctx context.Context, domainID string) (StoreDomain, error) {
	if domainID == "" {
		return StoreDomain{}, ErrInvalidInput
	}

	var disabled StoreDomain
	err := r.withTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var target StoreDomain
		err := tx.QueryRow(ctx, `
			SELECT id, store_id, domain, is_primary, verified_at, status, domain_type, verification_token, last_checked_at, created_at, updated_at
			FROM store_domains
			WHERE id = $1
			FOR UPDATE
		`, domainID).Scan(
			&target.ID, &target.StoreID, &target.Domain, &target.IsPrimary, &target.VerifiedAt, &target.Status, &target.DomainType, &target.VerificationToken, &target.LastCheckedAt, &target.CreatedAt, &target.UpdatedAt,
		)
		if err != nil {
			return translatePGError(err, "get domain to disable")
		}

		wasPrimary := target.IsPrimary
		storeID := target.StoreID

		err = tx.QueryRow(ctx, `
			UPDATE store_domains
			SET status = 'disabled',
			    is_primary = false,
			    updated_at = now()
			WHERE id = $1
			RETURNING id, store_id, domain, is_primary, verified_at, status, domain_type, verification_token, last_checked_at, created_at, updated_at
		`, domainID).Scan(
			&disabled.ID, &disabled.StoreID, &disabled.Domain, &disabled.IsPrimary, &disabled.VerifiedAt, &disabled.Status, &disabled.DomainType, &disabled.VerificationToken, &disabled.LastCheckedAt, &disabled.CreatedAt, &disabled.UpdatedAt,
		)
		if err != nil {
			return translatePGError(err, "disable domain")
		}

		// If the disabled domain was primary, fallback to an active platform domain if available.
		if wasPrimary {
			var fallbackID string
			err := tx.QueryRow(ctx, `
				SELECT id
				FROM store_domains
				WHERE store_id = $1 AND domain_type = 'platform' AND status = 'active'
				ORDER BY created_at ASC
				LIMIT 1
				FOR UPDATE
			`, storeID).Scan(&fallbackID)
			if err == nil && fallbackID != "" {
				if _, err := tx.Exec(ctx, `
					UPDATE store_domains
					SET is_primary = true, updated_at = now()
					WHERE id = $1
				`, fallbackID); err != nil {
					return translatePGError(err, "promote fallback platform primary")
				}
			}
			// If no fallback platform domain exists, store remains with no primary domain.
		}
		return nil
	})

	return disabled, err
}

// EnableDomainTx re-enables a domain according to moderation rules.
// For custom domains: restores to 'verified' (if verified_at is set) or 'pending' (if unverified), non-primary.
// For platform domains: restores to 'active'. If the store has no primary domain, promotes it to primary.
func (r Repository) EnableDomainTx(ctx context.Context, domainID string) (StoreDomain, error) {
	if domainID == "" {
		return StoreDomain{}, ErrInvalidInput
	}

	var enabled StoreDomain
	err := r.withTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var target StoreDomain
		err := tx.QueryRow(ctx, `
			SELECT id, store_id, domain, is_primary, verified_at, status, domain_type, verification_token, last_checked_at, created_at, updated_at
			FROM store_domains
			WHERE id = $1
			FOR UPDATE
		`, domainID).Scan(
			&target.ID, &target.StoreID, &target.Domain, &target.IsPrimary, &target.VerifiedAt, &target.Status, &target.DomainType, &target.VerificationToken, &target.LastCheckedAt, &target.CreatedAt, &target.UpdatedAt,
		)
		if err != nil {
			return translatePGError(err, "get domain to enable")
		}

		newStatus := "active"
		newIsPrimary := false

		if target.DomainType == "custom" {
			if target.VerifiedAt != nil {
				newStatus = "verified"
			} else {
				newStatus = "pending"
			}
			newIsPrimary = false
		} else if target.DomainType == "platform" {
			newStatus = "active"
			var primaryCount int
			if err := tx.QueryRow(ctx, `
				SELECT count(*)
				FROM store_domains
				WHERE store_id = $1 AND is_primary = true
			`, target.StoreID).Scan(&primaryCount); err != nil {
				return translatePGError(err, "count primary domains")
			}
			if primaryCount == 0 {
				newIsPrimary = true
			}
		}

		err = tx.QueryRow(ctx, `
			UPDATE store_domains
			SET status = $2,
			    is_primary = $3,
			    updated_at = now()
			WHERE id = $1
			RETURNING id, store_id, domain, is_primary, verified_at, status, domain_type, verification_token, last_checked_at, created_at, updated_at
		`, domainID, newStatus, newIsPrimary).Scan(
			&enabled.ID, &enabled.StoreID, &enabled.Domain, &enabled.IsPrimary, &enabled.VerifiedAt, &enabled.Status, &enabled.DomainType, &enabled.VerificationToken, &enabled.LastCheckedAt, &enabled.CreatedAt, &enabled.UpdatedAt,
		)
		if err != nil {
			return translatePGError(err, "enable domain")
		}
		return nil
	})

	return enabled, err
}

func scanStoreDomains(rows pgx.Rows) ([]StoreDomain, error) {
	var items []StoreDomain
	for rows.Next() {
		var item StoreDomain
		if err := rows.Scan(
			&item.ID, &item.StoreID, &item.Domain, &item.IsPrimary, &item.VerifiedAt, &item.Status, &item.DomainType, &item.VerificationToken, &item.LastCheckedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan store domain: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
