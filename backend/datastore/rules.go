package datastore

import (
	"context"
	"fmt"
	"net/url"

	log "github.com/go-pkgz/lgr"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RulesDAO data-access obj for custom parsing rules, implements Rules
type RulesDAO struct {
	*mongo.Collection
}

// Rule record, entry in mongo
type Rule struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Domain    string             `json:"domain"`
	MatchURLs []string           `json:"match_url,omitempty" bson:"match_urls,omitempty"`
	Content   string             `json:"content"`
	Author    string             `json:"author,omitempty" bson:"author,omitempty"`
	TS        string             `json:"ts,omitempty" bson:"ts,omitempty"` // ts of original article
	Excludes  []string           `json:"excludes,omitempty" bson:"excludes,omitempty"`
	TestURLs  []string           `json:"test_urls,omitempty" bson:"test_urls"`
	User      string             `json:"user"`
	Enabled   bool               `json:"enabled"`
}

// Get rule by url. Checks if found in mongo, matching by domain
func (r RulesDAO) Get(ctx context.Context, rURL string) (Rule, bool) {
	u, err := url.Parse(rURL)
	if err != nil {
		log.Printf("[WARN] failed to parse url=%s, error=%v", rURL, err)
		return Rule{}, false
	}

	var rules []Rule
	q := bson.M{"domain": u.Host, "enabled": true}
	log.Printf("[DEBUG] query %v", q)
	cursor, err := r.Find(ctx, q)
	if err != nil {
		log.Printf("[DEBUG] error looking for rules for %s", rURL)
		return Rule{}, false
	}
	if err := cursor.All(ctx, &rules); err != nil || len(rules) == 0 {
		log.Printf("[DEBUG] no custom rule for %s", rURL)
		return Rule{}, false
	}
	result := rules[0]
	log.Printf("[INFO] found rule for %s = [%v]", rURL, result)
	return result, true
}

// GetByID returns record by id
func (r RulesDAO) GetByID(ctx context.Context, id primitive.ObjectID) (Rule, bool) {
	var rule Rule
	err := r.Collection.FindOne(ctx, bson.M{"_id": id}).Decode(&rule)
	return rule, err == nil
}

// Save upsert rule
func (r RulesDAO) Save(ctx context.Context, rule Rule) (Rule, error) {
	ch, err := r.UpdateOne(ctx, bson.M{"domain": rule.Domain}, bson.M{"$set": rule}, options.Update().SetUpsert(true))
	if err != nil {
		log.Printf("[WARN] failed to save, error=%v, article=%v", err, rule)
		return rule, err
	}
	if ch.UpsertedID != nil {
		if oid, ok := ch.UpsertedID.(primitive.ObjectID); ok {
			rule.ID = oid
		}
	}
	// if rule was updated, we have no id, so try to find it by domain
	if rule.ID == primitive.NilObjectID {
		var found Rule
		err = r.Collection.FindOne(ctx, bson.M{"domain": rule.Domain}).Decode(&found)
		if err == nil {
			rule.ID = found.ID
		}
	}
	return rule, err
}

// Disable marks enabled=false, by id
func (r RulesDAO) Disable(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"enabled": false}})
	return err
}

// All returns list of all rules, both enabled and disabled
func (r RulesDAO) All(ctx context.Context) []Rule {
	cursor, err := r.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("[WARN] failed to retrieve all rules, error=%v", err)
		return []Rule{}
	}
	var result []Rule
	if err = cursor.All(ctx, &result); err != nil {
		log.Printf("[WARN] failed to retrieve all rules, error=%v", err)
		return []Rule{}
	}
	return result
}

func (s Rule) String() string {
	return fmt.Sprintf("{id=%s, domain=%s, content=%s, enabled=%v}", s.ID.Hex(), s.Domain, s.Content, s.Enabled)
}
