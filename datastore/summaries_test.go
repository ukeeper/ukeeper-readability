package datastore

import (
	"context"
	"testing"
	"time"

	"github.com/go-pkgz/testutils/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummariesSaveAndGet(t *testing.T) {
	summaries := setupStores(t).Summaries
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	t.Run("save and get", func(t *testing.T) {
		s := Summary{ID: "id-1", Summary: "short version", Model: "gpt-6-luna", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
		require.NoError(t, summaries.Save(ctx, s))
		got, ok := summaries.Get(ctx, "id-1")
		require.True(t, ok)
		assert.Equal(t, "short version", got.Summary)
		assert.Equal(t, "gpt-6-luna", got.Model)
		assert.True(t, got.CreatedAt.Equal(now))
	})

	t.Run("save replaces existing", func(t *testing.T) {
		require.NoError(t, summaries.Save(ctx, Summary{ID: "id-2", Summary: "old", ExpiresAt: now.Add(time.Hour)}))
		require.NoError(t, summaries.Save(ctx, Summary{ID: "id-2", Summary: "new", ExpiresAt: now.Add(time.Hour)}))
		got, ok := summaries.Get(ctx, "id-2")
		require.True(t, ok)
		assert.Equal(t, "new", got.Summary)
	})

	t.Run("missing", func(t *testing.T) {
		_, ok := summaries.Get(ctx, "no-such-id")
		assert.False(t, ok)
	})

	t.Run("expired is not returned", func(t *testing.T) {
		require.NoError(t, summaries.Save(ctx, Summary{ID: "id-3", Summary: "stale", ExpiresAt: now.Add(-time.Minute)}))
		_, ok := summaries.Get(ctx, "id-3")
		assert.False(t, ok)
	})
}

func TestSummariesTTLIndex(t *testing.T) {
	summaries := setupStores(t).Summaries
	cursor, err := summaries.Indexes().List(context.Background())
	require.NoError(t, err)
	var indexes []struct {
		Key                map[string]int `bson:"key"`
		ExpireAfterSeconds *int           `bson:"expireAfterSeconds"`
	}
	require.NoError(t, cursor.All(context.Background(), &indexes))

	for _, idx := range indexes {
		if len(idx.Key) != 1 || idx.Key[fieldExpiresAt] != 1 {
			continue
		}
		require.NotNil(t, idx.ExpireAfterSeconds, "index on %s is not a TTL index", fieldExpiresAt)
		assert.Equal(t, 0, *idx.ExpireAfterSeconds)
		return
	}
	t.Fatalf("no TTL index on %s, indexes=%v", fieldExpiresAt, indexes)
}

func setupStores(t *testing.T) Stores {
	t.Helper()
	mc := containers.NewMongoTestContainer(context.Background(), t, 5)
	t.Cleanup(func() { mc.Close(context.Background()) }) //nolint:errcheck

	server, err := New(mc.URI, "test_ureadability", 0)
	require.NoError(t, err)
	return server.GetStores()
}
