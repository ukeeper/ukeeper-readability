// Package datastore provides mongo implementation for store to keep and access rules
package datastore

import (
	"context"
	"errors"
	"time"

	log "github.com/go-pkgz/lgr"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoServer top level mongo ops
type MongoServer struct {
	client *mongo.Client
	dbName string
}

// New MongoServer
func New(connectionURI, dbName string, delay time.Duration) (*MongoServer, error) {
	log.Printf("[INFO] connect to mongo server with db=%s, delay=%s", dbName, delay)
	if delay > 0 {
		log.Printf("[DEBUG] initial mongo delay=%s", delay)
		time.Sleep(delay)
	}
	if connectionURI == "" {
		return nil, errors.New("env MONGO_URI not defined and --mongo not passed")
	}

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(connectionURI))
	if err != nil {
		return nil, err
	}

	return &MongoServer{client: client, dbName: dbName}, nil
}

// GetStores initialize collections and make indexes
func (m *MongoServer) GetStores() (rules RulesDAO) {
	rIndexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "enabled", Value: 1}, {Key: "domain", Value: 1}}},
		{Keys: bson.D{{Key: "user", Value: 1}, {Key: "domain", Value: 1}, {Key: "enabled", Value: 1}}},
		{Keys: bson.D{{Key: "domain", Value: 1}, {Key: "match_urls", Value: 1}}},
	}
	rules = RulesDAO{Collection: m.collection("rules", rIndexes)}
	return rules
}

// collection makes collection with indexes
func (m *MongoServer) collection(collection string, indexes []mongo.IndexModel) *mongo.Collection {
	log.Printf("[INFO] create collection %s.%s", m.dbName, collection)
	coll := m.client.Database(m.dbName).Collection(collection)

	if _, err := coll.Indexes().CreateMany(context.Background(), indexes); err != nil {
		log.Printf("[WARN] can't ensure indexes=%v, error=%v", indexes, err)
	}
	return coll
}
