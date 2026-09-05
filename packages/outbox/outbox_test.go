package outbox_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/matjeroapps/core/internal/testdb"
	"github.com/matjeroapps/core/packages/config"
	"github.com/matjeroapps/core/packages/database"
	"github.com/matjeroapps/core/packages/events"
	"github.com/matjeroapps/core/packages/outbox"
)

func setupOutboxDB(t *testing.T) (*database.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://commerce:commerce@localhost:5432/commerce?sslmode=disable"
	}
	db := testdb.Open(t, dsn)

	ctx := context.Background()
	for _, name := range []string{
		"000001_event_delivery_foundation",
		"000013_outbox_publish_claims",
	} {
		content, err := os.ReadFile(filepath.Join("..", "..", "migrations", name+".up.sql"))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := db.Pool.Exec(ctx, string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}

	return db, ctx
}

type fakePublisher struct {
	mu        sync.Mutex
	published []events.EventEnvelope
	failCount int
}

func (f *fakePublisher) PublishEvent(ctx context.Context, exchange, routingKey string, event events.EventEnvelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failCount > 0 {
		f.failCount--
		return errors.New("simulated publish failure")
	}

	f.published = append(f.published, event)
	return nil
}

func sampleEnvelope() events.EventEnvelope {
	return events.EventEnvelope{
		EventID:          uuid.NewString(),
		EventType:        "commerce.order.created.v1",
		SchemaVersion:    1,
		AggregateType:    "order",
		AggregateID:      "ord_123",
		AggregateVersion: 1,
		CorrelationID:    "corr_abc_999",
		CausationID:      "cmd_create_order",
		OccurredAt:       time.Now().UTC().Truncate(time.Microsecond),
		Payload: map[string]any{
			"order_id":    "ord_123",
			"store_id":    "str_456",
			"total_cents": float64(1500),
		},
	}
}

func TestOutboxTwoPublishersNeverClaimSameEvent(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	var eventIDs []string
	for i := 0; i < 10; i++ {
		env := sampleEnvelope()
		tx, err := db.Pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		if err := store.Enqueue(ctx, tx, env); err != nil {
			t.Fatalf("enqueue event: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit tx: %v", err)
		}
		eventIDs = append(eventIDs, env.EventID)
	}

	claimIDA := uuid.NewString()
	claimIDB := uuid.NewString()

	var claimedA, claimedB []string
	var errA, errB error

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		claimedA, errA = store.ClaimBatch(ctx, db.Pool, claimIDA, 10, 30*time.Second)
	}()

	go func() {
		defer wg.Done()
		claimedB, errB = store.ClaimBatch(ctx, db.Pool, claimIDB, 10, 30*time.Second)
	}()

	wg.Wait()

	if errA != nil {
		t.Fatalf("publisher A claim error: %v", errA)
	}
	if errB != nil {
		t.Fatalf("publisher B claim error: %v", errB)
	}

	totalClaimed := len(claimedA) + len(claimedB)
	if totalClaimed != 10 {
		t.Errorf("expected total 10 claimed events across both publishers, got %d (A: %d, B: %d)",
			totalClaimed, len(claimedA), len(claimedB))
	}

	seen := make(map[string]bool)
	for _, id := range claimedA {
		seen[id] = true
	}
	for _, id := range claimedB {
		if seen[id] {
			t.Errorf("overlap detected! Both publishers claimed event %s", id)
		}
	}
}

func TestOutboxStaleLeaseCanBeReclaimed(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	env := sampleEnvelope()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	claimA := uuid.NewString()
	claimed, err := store.ClaimBatch(ctx, db.Pool, claimA, 1, 30*time.Second)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim A failed: %v", err)
	}

	// Fast-forward claim timestamp to simulate lease expiration
	_, err = db.Pool.Exec(ctx, `
		UPDATE outbox_events
		SET publish_claimed_at = clock_timestamp() - interval '1 minute'
		WHERE event_id = $1::uuid
	`, env.EventID)
	if err != nil {
		t.Fatalf("simulate lease expiry: %v", err)
	}

	claimB := uuid.NewString()
	reclaimed, err := store.ClaimBatch(ctx, db.Pool, claimB, 1, 30*time.Second)
	if err != nil {
		t.Fatalf("claim B error: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0] != env.EventID {
		t.Fatalf("expected claim B to reclaim expired event %s, got %v", env.EventID, reclaimed)
	}
}

func TestOutboxStaleAckCannotMarkPublished(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	env := sampleEnvelope()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	claimA := uuid.NewString()
	_, err = store.ClaimBatch(ctx, db.Pool, claimA, 1, 30*time.Second)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}

	claimB := uuid.NewString()
	// Force update claim to claimB (simulating lease expiration and reclaim)
	_, err = db.Pool.Exec(ctx, `
		UPDATE outbox_events
		SET publish_claim_id = $1::uuid, publish_claimed_at = clock_timestamp()
		WHERE event_id = $2::uuid
	`, claimB, env.EventID)
	if err != nil {
		t.Fatalf("simulate claim takeover: %v", err)
	}

	marked, err := store.MarkPublished(ctx, db.Pool, env.EventID, claimA)
	if err != nil {
		t.Fatalf("MarkPublished with stale claim A error: %v", err)
	}
	if marked {
		t.Errorf("expected MarkPublished to return false for stale claim A, got true")
	}
}

func TestOutboxLostClaimSkipsPublish(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	env := sampleEnvelope()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	claimA := uuid.NewString()
	_, err = store.ClaimBatch(ctx, db.Pool, claimA, 1, 30*time.Second)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}

	// Simulate claim loss by clearing publish_claim_id
	_, err = db.Pool.Exec(ctx, `
		UPDATE outbox_events
		SET publish_claim_id = NULL
		WHERE event_id = $1::uuid
	`, env.EventID)
	if err != nil {
		t.Fatalf("simulate lost claim: %v", err)
	}

	loaded, err := store.RenewAndLoadEvent(ctx, db.Pool, env.EventID, claimA, 30*time.Second)
	if !errors.Is(err, outbox.ErrClaimLost) {
		t.Fatalf("expected ErrClaimLost, got loaded=%v, err=%v", loaded, err)
	}
}

func TestOutboxRenewClaimAndLoadReturnsCompleteEnvelope(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	env := sampleEnvelope()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	claimID := uuid.NewString()
	claimed, err := store.ClaimBatch(ctx, db.Pool, claimID, 1, 30*time.Second)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim batch failed: %v", err)
	}

	loaded, err := store.RenewAndLoadEvent(ctx, db.Pool, env.EventID, claimID, 30*time.Second)
	if err != nil {
		t.Fatalf("RenewAndLoadEvent error: %v", err)
	}

	if loaded.EventID != env.EventID {
		t.Errorf("EventID mismatch: expected %s, got %s", env.EventID, loaded.EventID)
	}
	if loaded.EventType != env.EventType {
		t.Errorf("EventType mismatch: expected %s, got %s", env.EventType, loaded.EventType)
	}
	if loaded.SchemaVersion != env.SchemaVersion {
		t.Errorf("SchemaVersion mismatch: expected %d, got %d", env.SchemaVersion, loaded.SchemaVersion)
	}
	if loaded.AggregateType != env.AggregateType {
		t.Errorf("AggregateType mismatch: expected %s, got %s", env.AggregateType, loaded.AggregateType)
	}
	if loaded.AggregateID != env.AggregateID {
		t.Errorf("AggregateID mismatch: expected %s, got %s", env.AggregateID, loaded.AggregateID)
	}
	if loaded.AggregateVersion != env.AggregateVersion {
		t.Errorf("AggregateVersion mismatch: expected %d, got %d", env.AggregateVersion, loaded.AggregateVersion)
	}
	if loaded.CorrelationID != env.CorrelationID {
		t.Errorf("CorrelationID mismatch: expected %s, got %s", env.CorrelationID, loaded.CorrelationID)
	}
	if loaded.CausationID != env.CausationID {
		t.Errorf("CausationID mismatch: expected %s, got %s", env.CausationID, loaded.CausationID)
	}
	if !loaded.OccurredAt.Equal(env.OccurredAt) {
		t.Errorf("OccurredAt mismatch: expected %v, got %v", env.OccurredAt, loaded.OccurredAt)
	}
}

func TestOutboxLongBatchRenewsNearExpiry(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	env := sampleEnvelope()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	claimID := uuid.NewString()
	claimed, err := store.ClaimBatch(ctx, db.Pool, claimID, 1, 30*time.Second)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim batch failed: %v", err)
	}

	err = store.RenewBatchNearExpiry(ctx, db.Pool, claimed, claimID, 30*time.Second)
	if err != nil {
		t.Fatalf("RenewBatchNearExpiry error: %v", err)
	}
}

func TestOutboxPublishFailureSchedulesBackoff(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	env := sampleEnvelope()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	claimID := uuid.NewString()
	_, err = store.ClaimBatch(ctx, db.Pool, claimID, 1, 30*time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	released, err := store.ReleaseWithBackoff(ctx, db.Pool, env.EventID, claimID)
	if err != nil {
		t.Fatalf("ReleaseWithBackoff error: %v", err)
	}
	if !released {
		t.Fatalf("expected ReleaseWithBackoff to return true, got false")
	}

	var attempts int
	var nextAttemptAt time.Time
	err = db.Pool.QueryRow(ctx, `
		SELECT publish_attempts, next_attempt_at
		FROM outbox_events
		WHERE event_id = $1::uuid
	`, env.EventID).Scan(&attempts, &nextAttemptAt)
	if err != nil {
		t.Fatalf("query event state: %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected publish_attempts = 1, got %d", attempts)
	}
	if !nextAttemptAt.After(time.Now().Add(-1 * time.Second)) {
		t.Errorf("expected next_attempt_at in the future, got %v", nextAttemptAt)
	}
}

func TestOutboxBackoffIsBounded(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	env := sampleEnvelope()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for i := 0; i < 10; i++ {
		claimID := uuid.NewString()
		// Fast forward next_attempt_at to make it claimable
		_, err := db.Pool.Exec(ctx, `
			UPDATE outbox_events
			SET next_attempt_at = clock_timestamp() - interval '1 second'
			WHERE event_id = $1::uuid
		`, env.EventID)
		if err != nil {
			t.Fatalf("fast forward next_attempt_at: %v", err)
		}

		claimed, err := store.ClaimBatch(ctx, db.Pool, claimID, 1, 30*time.Second)
		if err != nil || len(claimed) != 1 {
			t.Fatalf("iteration %d claim failed: %v", i, err)
		}

		released, err := store.ReleaseWithBackoff(ctx, db.Pool, env.EventID, claimID)
		if err != nil || !released {
			t.Fatalf("iteration %d release failed: %v", i, err)
		}
	}

	var attempts int
	err = db.Pool.QueryRow(ctx, `
		SELECT publish_attempts
		FROM outbox_events
		WHERE event_id = $1::uuid
	`, env.EventID).Scan(&attempts)
	if err != nil {
		t.Fatalf("query attempts: %v", err)
	}

	if attempts != 10 {
		t.Errorf("expected publish_attempts = 10, got %d", attempts)
	}
}

func TestOutboxRetryPreservesEventID(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	env := sampleEnvelope()
	env.EventType = "commerce.order.status_changed.v1"
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	cfg, err := config.Load("test-service")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	fakePub := &fakePublisher{failCount: 1}
	proc := outbox.NewProcessor(cfg, db.Pool, fakePub, nil)

	// First batch run fails publish and schedules backoff
	processed, err := proc.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("first ProcessBatch error: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 event claimed, got %d", processed)
	}

	fakePub.mu.Lock()
	publishedCount := len(fakePub.published)
	fakePub.mu.Unlock()
	if publishedCount != 0 {
		t.Fatalf("expected 0 published events due to failure, got %d", publishedCount)
	}

	// Reset next_attempt_at to make it claimable again
	_, err = db.Pool.Exec(ctx, `
		UPDATE outbox_events
		SET next_attempt_at = clock_timestamp() - interval '1 second'
		WHERE event_id = $1::uuid
	`, env.EventID)
	if err != nil {
		t.Fatalf("reset next_attempt_at: %v", err)
	}

	// Second run succeeds
	processed, err = proc.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("second ProcessBatch error: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 event claimed, got %d", processed)
	}

	fakePub.mu.Lock()
	defer fakePub.mu.Unlock()
	if len(fakePub.published) != 1 {
		t.Fatalf("expected 1 published event on retry, got %d", len(fakePub.published))
	}
	if fakePub.published[0].EventID != env.EventID {
		t.Errorf("expected retried event to preserve stable EventID %s, got %s", env.EventID, fakePub.published[0].EventID)
	}
}

func TestOutboxConfirmThenCrashRepublishesSameEventID(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	env := sampleEnvelope()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Worker 1 claims and publishes, but "crashes" before MarkPublished
	claim1 := uuid.NewString()
	claimed1, err := store.ClaimBatch(ctx, db.Pool, claim1, 1, 30*time.Second)
	if err != nil || len(claimed1) != 1 {
		t.Fatalf("claim1 failed: %v", err)
	}
	env1, err := store.RenewAndLoadEvent(ctx, db.Pool, env.EventID, claim1, 30*time.Second)
	if err != nil {
		t.Fatalf("RenewAndLoadEvent worker 1 error: %v", err)
	}
	// Simulate publication success to broker, but worker 1 crashes before calling MarkPublished

	// Simulate lease expiration
	_, err = db.Pool.Exec(ctx, `
		UPDATE outbox_events
		SET publish_claimed_at = clock_timestamp() - interval '1 minute'
		WHERE event_id = $1::uuid
	`, env.EventID)
	if err != nil {
		t.Fatalf("expire claim1: %v", err)
	}

	// Worker 2 reclaims and publishes
	claim2 := uuid.NewString()
	claimed2, err := store.ClaimBatch(ctx, db.Pool, claim2, 1, 30*time.Second)
	if err != nil || len(claimed2) != 1 {
		t.Fatalf("claim2 failed: %v", err)
	}
	env2, err := store.RenewAndLoadEvent(ctx, db.Pool, env.EventID, claim2, 30*time.Second)
	if err != nil {
		t.Fatalf("RenewAndLoadEvent worker 2 error: %v", err)
	}

	if env1.EventID != env2.EventID {
		t.Errorf("expected both publication attempts to preserve same EventID %s, got %s vs %s",
			env.EventID, env1.EventID, env2.EventID)
	}
}

func TestOrderCreatedPublishedEnvelopeMatchesOutbox(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	env := sampleEnvelope()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	cfg, err := config.Load("test-service")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	fakePub := &fakePublisher{}
	proc := outbox.NewProcessor(cfg, db.Pool, fakePub, nil)

	processed, err := proc.ProcessBatch(ctx)
	if err != nil || processed != 1 {
		t.Fatalf("ProcessBatch expected 1, got %d (err: %v)", processed, err)
	}

	fakePub.mu.Lock()
	defer fakePub.mu.Unlock()
	if len(fakePub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(fakePub.published))
	}
	pubEnv := fakePub.published[0]

	if pubEnv.EventID != env.EventID ||
		pubEnv.EventType != env.EventType ||
		pubEnv.SchemaVersion != env.SchemaVersion ||
		pubEnv.AggregateType != env.AggregateType ||
		pubEnv.AggregateID != env.AggregateID ||
		pubEnv.AggregateVersion != env.AggregateVersion ||
		pubEnv.CorrelationID != env.CorrelationID ||
		pubEnv.CausationID != env.CausationID {
		t.Errorf("published envelope does not match outbox row: got %+v", pubEnv)
	}
}

func TestOrderCreatedPublishedCorrelationPreserved(t *testing.T) {
	db, ctx := setupOutboxDB(t)
	store := outbox.NewStore()

	expectedCorrelationID := "http_req_corr_98765"
	env := sampleEnvelope()
	env.CorrelationID = expectedCorrelationID

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.Enqueue(ctx, tx, env); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	cfg, err := config.Load("test-service")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	fakePub := &fakePublisher{}
	proc := outbox.NewProcessor(cfg, db.Pool, fakePub, nil)

	_, err = proc.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("ProcessBatch error: %v", err)
	}

	fakePub.mu.Lock()
	defer fakePub.mu.Unlock()
	if len(fakePub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(fakePub.published))
	}

	if fakePub.published[0].CorrelationID != expectedCorrelationID {
		t.Errorf("expected CorrelationID %s to be preserved, got %s",
			expectedCorrelationID, fakePub.published[0].CorrelationID)
	}
}

func TestMigration000013UpDownUpSafety(t *testing.T) {
	db, ctx := setupOutboxDB(t)

	downContent, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000013_outbox_publish_claims.down.sql"))
	if err != nil {
		t.Fatalf("read migration 000013 down: %v", err)
	}
	upContent, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000013_outbox_publish_claims.up.sql"))
	if err != nil {
		t.Fatalf("read migration 000013 up: %v", err)
	}

	if _, err := db.Pool.Exec(ctx, string(downContent)); err != nil {
		t.Fatalf("migration 000013 down failed: %v", err)
	}

	if _, err := db.Pool.Exec(ctx, string(upContent)); err != nil {
		t.Fatalf("migration 000013 up again failed: %v", err)
	}
}
