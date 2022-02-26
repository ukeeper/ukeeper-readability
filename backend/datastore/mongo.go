// Package datastore provides mongo implementation for store to keep and access rules
package datastore

import (
	"context"
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
func New(address, password, dbName string, delay int) (res *MongoServer) {
	log.Printf("[INFO] make new mongo server with ip=%s, db=%s, delay=%dsecs", address, dbName, delay)
	if delay > 0 {
		log.Printf("[DEBUG] initial mongo delay=%d", delay)
		time.Sleep(time.Duration(delay) * time.Second)
	}
	if address == "" {
		log.Fatalf("[ERROR] env MONGO not defined and --mongo not passed")
	}

	log.Printf("[DEBUG] get mongo for %s", address)
	connectString := "mongodb://root:" + password + "@" + address
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(connectString))
	if err != nil {
		log.Fatalf("[ERROR] can't connect to mongo %v", err)
	}

	return &MongoServer{client: client, dbName: dbName}
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

	for _, index := range indexes {
		if _, err := coll.Indexes().CreateOne(context.Background(), index); err != nil {
			log.Printf("[WARN] can't ensure index=%v, error=%v", index, err)
		}
	}
	return coll
}
