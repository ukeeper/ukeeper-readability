package datastore

import (
	"context"
	"errors"
	"fmt"
	"time"

	log "github.com/go-pkgz/lgr"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Summary is a cached article summary, keyed by a hash of the summarized input
type Summary struct {
	ID        string    `bson:"_id"`
	Summary   string    `bson:"summary"`
	Model     string    `bson:"model"`
	CreatedAt time.Time `bson:"created_at"`
	ExpiresAt time.Time `bson:"expires_at"` // mongo TTL index on this field deletes the document
}

// SummariesDAO data-access obj for cached summaries, implements Summaries
type SummariesDAO struct {
	*mongo.Collection
}

// Get returns an unexpired summary by id
func (s SummariesDAO) Get(ctx context.Context, id string) (Summary, bool) {
	var res Summary
	// the TTL monitor runs about once a minute, so expired documents can still be present
	err := s.FindOne(ctx, bson.M{fieldID: id, fieldExpiresAt: bson.M{"$gt": time.Now()}}).Decode(&res)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			log.Printf("[WARN] can't get summary %s, error=%v", id, err)
		}
		return Summary{}, false
	}
	return res, true
}

// Save upserts summary by id
func (s SummariesDAO) Save(ctx context.Context, summary Summary) error {
	if _, err := s.ReplaceOne(ctx, bson.M{fieldID: summary.ID}, summary, options.Replace().SetUpsert(true)); err != nil {
		return fmt.Errorf("save summary %s: %w", summary.ID, err)
	}
	return nil
}
