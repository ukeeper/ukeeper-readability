// Package datastore provides mongo implementation for store to keep and access rules
package datastore

import (
	"time"

	"github.com/globalsign/mgo"
	log "github.com/go-pkgz/lgr"
)

// MongoServer top level mongo ops
type MongoServer struct {
	address string
	creds   *mgo.Credential
	session *mgo.Session
	dbName  string
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
	session, err := mgo.Dial(address)
	if err != nil {
		log.Fatalf("[ERROR] can't connect to mongo %v", err)
	}
	session.SetMode(mgo.Monotonic, true)

	creds := &mgo.Credential{Username: "root", Password: password}
	if password != "" {
		log.Print("[DEBUG] login to mongo")
		if err = session.Login(creds); err != nil {
			log.Fatalf("[ERROR] can't login to mongo %v", err)
		}
	}
	return &MongoServer{address: address, creds: creds, session: session, dbName: dbName}
}

// GetStores initialize collections and make indexes
func (m *MongoServer) GetStores() (rules RulesDAO) {
	rIndexes := []mgo.Index{
		{Key: []string{"enabled", "domain"}},
		{Key: []string{"user", "domain", "enabled"}},
		{Key: []string{"domain", "match_urls"}},
	}
	rules = RulesDAO{Collection: m.collection("rules", rIndexes)}
	return rules
}

// collection makes collection with indexes
func (m *MongoServer) collection(collection string, indexes []mgo.Index) *mgo.Collection {
	log.Printf("[INFO] create collection %s.%s", m.dbName, collection)
	coll := m.session.DB(m.dbName).C(collection)
	if err := coll.Create(&mgo.CollectionInfo{ForceIdIndex: true}); err != nil {
		log.Printf("[WARN] can't create collection %s, error=%v", collection, err)
	}

	for _, index := range indexes {
		if err := coll.EnsureIndex(index); err != nil {
			log.Printf("[WARN] can't ensure index=%v, error=%v", index, err)
		}
	}
	return coll
}
