package datastore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummariesDAO_SaveAndGet(t *testing.T) {
	if _, ok := os.LookupEnv("ENABLE_MONGO_TESTS"); !ok {
		t.Skip("ENABLE_MONGO_TESTS env variable is not set")
	}

	mdb, err := New("mongodb://localhost:27017", "test_ureadability", 0)
	require.NoError(t, err)

	// Create a unique collection for this test to avoid conflicts
	collection := mdb.client.Database(mdb.dbName).Collection("summaries_test")
	defer func() {
		_ = collection.Drop(context.Background())
	}()

	dao := SummariesDAO{Collection: collection}

	content := "This is a test article content. It should generate a unique hash."
	summary := Summary{
		Content:   content,
		Summary:   "This is a test summary of the article.",
		Model:     "gpt-4o-mini",
		CreatedAt: time.Now(),
	}

	// Test saving a summary
	err = dao.Save(context.Background(), summary)
	require.NoError(t, err)

	// Test getting the summary
	foundSummary, found := dao.Get(context.Background(), content)
	assert.True(t, found)
	assert.Equal(t, summary.Summary, foundSummary.Summary)
	assert.Equal(t, summary.Model, foundSummary.Model)
	assert.NotEmpty(t, foundSummary.ID)

	// Test getting a non-existent summary
	_, found = dao.Get(context.Background(), "non-existent content")
	assert.False(t, found)

	// Test updating an existing summary
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

	// Test deleting a summary
	err = dao.Delete(context.Background(), foundSummary.ID)
	require.NoError(t, err)

	_, found = dao.Get(context.Background(), content)
	assert.False(t, found)
}

func TestGenerateContentHash(t *testing.T) {
	content1 := "This is a test content."
	content2 := "This is a different test content."

	hash1 := generateContentHash(content1)
	hash2 := generateContentHash(content2)

	assert.NotEqual(t, hash1, hash2)
	assert.Equal(t, hash1, generateContentHash(content1)) // Same content should produce same hash
	assert.Equal(t, 64, len(hash1))                       // SHA-256 produces 64 character hex string
}
