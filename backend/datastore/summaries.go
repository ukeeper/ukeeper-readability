// Package datastore provides mongo implementation for store to keep and access summaries
package datastore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	log "github.com/go-pkgz/lgr"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Summary contains information about a cached summary
type Summary struct {
	ID        string    `bson:"_id"`     // SHA256 hash of the content
	Content   string    `bson:"content"` // Original content that was summarized (could be truncated for storage efficiency)
	Summary   string    `bson:"summary"` // Generated summary
	Model     string    `bson:"model"`   // OpenAI model used for summarization
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

// SummariesDAO handles database operations for article summaries
type SummariesDAO struct {
	Collection *mongo.Collection
}

// Get returns summary by content hash
func (s SummariesDAO) Get(ctx context.Context, content string) (Summary, bool) {
	contentHash := generateContentHash(content)
	res := s.Collection.FindOne(ctx, bson.M{"_id": contentHash})
	if res.Err() != nil {
		if res.Err() == mongo.ErrNoDocuments {
			return Summary{}, false
		}
		log.Printf("[WARN] can't get summary for hash %s: %v", contentHash, res.Err())
		return Summary{}, false
	}

	summary := Summary{}
	if err := res.Decode(&summary); err != nil {
		log.Printf("[WARN] can't decode summary document for hash %s: %v", contentHash, err)
		return Summary{}, false
	}

	return summary, true
}

// Save creates or updates summary in the database
func (s SummariesDAO) Save(ctx context.Context, summary Summary) error {
	if summary.ID == "" {
		summary.ID = generateContentHash(summary.Content)
	}

	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = time.Now()
	}
	summary.UpdatedAt = time.Now()

	opts := options.Update().SetUpsert(true)
	_, err := s.Collection.UpdateOne(
		ctx,
		bson.M{"_id": summary.ID},
		bson.M{"$set": summary},
		opts,
	)
	if err != nil {
		return fmt.Errorf("failed to save summary: %w", err)
	}
	return nil
}

// Delete removes summary from the database
func (s SummariesDAO) Delete(ctx context.Context, contentHash string) error {
	_, err := s.Collection.DeleteOne(ctx, bson.M{"_id": contentHash})
	if err != nil {
		return fmt.Errorf("failed to delete summary: %w", err)
	}
	return nil
}

// generateContentHash creates a hash for the content to use as an ID
func generateContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}
