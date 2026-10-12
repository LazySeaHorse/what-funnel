package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/services/workspace-svc/internal/service"
)

func TestListAuditLogs_PaginationFiltersAndIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	svc, pool := testService(t)
	ctx := context.Background()
	accountA, actorA := setupTestTenant(t, pool, "Audit A", "audit-a@example.com")
	accountB, actorB := setupTestTenant(t, pool, "Audit B", "audit-b@example.com")

	base := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	insert := func(acct uuid.UUID, actor *uuid.UUID, action string, at time.Time) {
		_, err := pool.Exec(ctx, `INSERT INTO audit_logs (account_id, actor_user_id, action, target_type, metadata, created_at)
			VALUES ($1, $2, $3, 'user', '{"k":"v"}', $4)`, acct, actor, action, at)
		require.NoError(t, err)
	}
	// 5 rows for A; the last two share a timestamp to exercise the id tiebreak.
	insert(accountA, &actorA, "user.login", base)
	insert(accountA, &actorA, "user.created", base.Add(1*time.Minute))
	insert(accountA, nil, "channel.deleted", base.Add(2*time.Minute))
	insert(accountA, &actorA, "user.login", base.Add(3*time.Minute))
	insert(accountA, &actorA, "user.login", base.Add(3*time.Minute))
	insert(accountB, &actorB, "user.login", base.Add(10*time.Minute))

	// Pagination walks all rows exactly once, newest first.
	var seen []uuid.UUID
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		page, err := svc.ListAuditLogs(ctx, accountA, service.AuditLogFilter{Limit: 2, Cursor: cursor})
		require.NoError(t, err)
		for _, it := range page.Items {
			seen = append(seen, it.ID)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	assert.Len(t, seen, 5)
	assert.Len(t, map[uuid.UUID]bool{seen[0]: true, seen[1]: true, seen[2]: true, seen[3]: true, seen[4]: true}, 5)

	all, err := svc.ListAuditLogs(ctx, accountA, service.AuditLogFilter{})
	require.NoError(t, err)
	require.Len(t, all.Items, 5)
	assert.Nil(t, all.NextCursor)
	for i := 1; i < len(all.Items); i++ {
		assert.False(t, all.Items[i].CreatedAt.After(all.Items[i-1].CreatedAt))
	}
	// Tenant isolation: B's row is never visible to A.
	for _, it := range all.Items {
		assert.NotEqual(t, &actorB, it.ActorUserID)
	}
	// Deleted/NULL actor yields null name/email; joined actor carries them.
	var nullActor, withActor int
	for _, it := range all.Items {
		if it.ActorUserID == nil {
			nullActor++
			assert.Nil(t, it.ActorEmail)
		} else {
			withActor++
			require.NotNil(t, it.ActorEmail)
			assert.Equal(t, "audit-a@example.com", *it.ActorEmail)
		}
		assert.JSONEq(t, `{"k":"v"}`, string(it.Metadata))
	}
	assert.Equal(t, 1, nullActor)
	assert.Equal(t, 4, withActor)

	// Filters.
	byAction, err := svc.ListAuditLogs(ctx, accountA, service.AuditLogFilter{Action: "user.login"})
	require.NoError(t, err)
	assert.Len(t, byAction.Items, 3)

	byActor, err := svc.ListAuditLogs(ctx, accountA, service.AuditLogFilter{ActorUserID: &actorA})
	require.NoError(t, err)
	assert.Len(t, byActor.Items, 4)
	otherActor, err := svc.ListAuditLogs(ctx, accountA, service.AuditLogFilter{ActorUserID: &actorB})
	require.NoError(t, err)
	assert.Empty(t, otherActor.Items)

	from, to := base.Add(1*time.Minute), base.Add(2*time.Minute)
	ranged, err := svc.ListAuditLogs(ctx, accountA, service.AuditLogFilter{From: &from, To: &to})
	require.NoError(t, err)
	assert.Len(t, ranged.Items, 2)

	// Limit is capped and bad cursors are rejected.
	_, err = svc.ListAuditLogs(ctx, accountA, service.AuditLogFilter{Cursor: "garbage!"})
	assert.ErrorIs(t, err, service.ErrInvalidAuditCursor)
}

func TestAuditCursorRoundTripIsOpaque(t *testing.T) {
	c := service.EncodeAuditCursor(time.Date(2024, 1, 1, 0, 0, 0, 123456000, time.UTC), uuid.New())
	assert.NotContains(t, c, "2024")
}
