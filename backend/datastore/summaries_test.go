package datastore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestSummariesDAO_SaveAndGet(t *testing.T) {
	if _, ok := os.LookupEnv("ENABLE_MONGO_TESTS"); !ok {
		t.Skip("ENABLE_MONGO_TESTS env variable is not set")
	}

	mdb, err := New("mongodb://localhost:27017", "test_ureadability", 0)
	require.NoError(t, err)

	// create a unique collection for this test to avoid conflicts
	collection := mdb.client.Database(mdb.dbName).Collection("summaries_test")
	defer func() {
		_ = collection.Drop(context.Background())
	}()

	// create an index on the expiresAt field
	_, err = collection.Indexes().CreateOne(context.Background(),
		mongo.IndexModel{
			Keys: bson.D{{"expires_at", 1}},
		})
	require.NoError(t, err)

	dao := SummariesDAO{Collection: collection}

	content := "This is a test article content. It should generate a unique hash."
	summary := Summary{
		Content:   content,
		Summary:   "This is a test summary of the article.",
		Model:     "gpt-4o-mini",
		CreatedAt: time.Now(),
	}

	// test saving a summary
	err = dao.Save(context.Background(), summary)
	require.NoError(t, err)

	// test getting the summary
	foundSummary, found := dao.Get(context.Background(), content)
	assert.True(t, found)
	assert.Equal(t, summary.Summary, foundSummary.Summary)
	assert.Equal(t, summary.Model, foundSummary.Model)
	assert.NotEmpty(t, foundSummary.ID)

	// test getting a non-existent summary
	_, found = dao.Get(context.Background(), "non-existent content")
	assert.False(t, found)

	// test updating an existing summary
	updatedSummary := Summary{
		ID:        foundSummary.ID,
		Content:   content,
		Summary:   "This is an updated summary.",
		Model:     "gpt-4o-mini",
		CreatedAt: foundSummary.CreatedAt,
	}

	err = dao.Save(context.Background(), updatedSummary)
	require.NoError(t, err)

	foundSummary, found = dao.Get(context.Background(), content)
	assert.True(t, found)
	assert.Equal(t, "This is an updated summary.", foundSummary.Summary)
	assert.Equal(t, updatedSummary.CreatedAt, foundSummary.CreatedAt)
	assert.NotEqual(t, updatedSummary.UpdatedAt, foundSummary.UpdatedAt) // UpdatedAt should be set by the DAO

	// test deleting a summary
	err = dao.Delete(context.Background(), foundSummary.ID)
	require.NoError(t, err)

	_, found = dao.Get(context.Background(), content)
	assert.False(t, found)
}

func TestGenerateContentHash(t *testing.T) {
	content1 := "This is a test content."
	content2 := "This is a different test content."

	hash1 := GenerateContentHash(content1)
	hash2 := GenerateContentHash(content2)

	assert.NotEqual(t, hash1, hash2)
	assert.Equal(t, hash1, GenerateContentHash(content1)) // same content should produce same hash
	assert.Equal(t, 64, len(hash1))                       // SHA-256 produces 64 character hex string
}

func TestSummariesDAO_CleanupExpired(t *testing.T) {
	if _, ok := os.LookupEnv("ENABLE_MONGO_TESTS"); !ok {
		t.Skip("ENABLE_MONGO_TESTS env variable is not set")
	}

	mdb, err := New("mongodb://localhost:27017", "test_ureadability", 0)
	require.NoError(t, err)

	// create a unique collection for this test to avoid conflicts
	collection := mdb.client.Database(mdb.dbName).Collection("summaries_expired_test")
	defer func() {
		_ = collection.Drop(context.Background())
	}()

	// create an index on the expiresAt field
	_, err = collection.Indexes().CreateOne(context.Background(),
		mongo.IndexModel{
			Keys: bson.D{{"expires_at", 1}},
		})
	require.NoError(t, err)

	dao := SummariesDAO{Collection: collection}
	ctx := context.Background()

	// add expired summary
	expiredSummary := Summary{
		Content:   "This is an expired summary",
		Summary:   "Expired content",
		Model:     "gpt-4o-mini",
		CreatedAt: time.Now().Add(-48 * time.Hour),
		UpdatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour), // expired 24 hours ago
	}
	err = dao.Save(ctx, expiredSummary)
	require.NoError(t, err)

	// add valid summary
	validSummary := Summary{
		Content:   "This is a valid summary",
		Summary:   "Valid content",
		Model:     "gpt-4o-mini",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // expires in 24 hours
	}
	err = dao.Save(ctx, validSummary)
	require.NoError(t, err)

	// verify both summaries exist
	_, foundExpired := dao.Get(ctx, expiredSummary.Content)
	assert.True(t, foundExpired, "Expected to find expired summary before cleanup")

	_, foundValid := dao.Get(ctx, validSummary.Content)
	assert.True(t, foundValid, "Expected to find valid summary before cleanup")

	// run cleanup
	count, err := dao.CleanupExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "Expected to clean up exactly one record")

	// verify expired summary is gone but valid remains
	_, foundExpired = dao.Get(ctx, expiredSummary.Content)
	assert.False(t, foundExpired, "Expected expired summary to be deleted")

	_, foundValid = dao.Get(ctx, validSummary.Content)
	assert.True(t, foundValid, "Expected valid summary to still exist")
}
