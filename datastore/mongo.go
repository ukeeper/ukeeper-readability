package datastore

import (
	"log"
	"time"

	"gopkg.in/mgo.v2"
)

//MongoServer top level mongo ops
type MongoServer struct {
	address string
	creds   *mgo.Credential
	session *mgo.Session
	dbName  string
}

//New MongoServer
func New(address string, password string, dbName string, delay int) (res *MongoServer) {
	log.Printf("make new mongo server with ip=%s, db=%s, delay=%dsecs", address, dbName, delay)
	if delay > 0 {
		log.Printf("initial mongo delay=%d", delay)
		time.Sleep(time.Duration(delay) * time.Second)
	}
	if address == "" {
		log.Fatal("env MONGO not defined and --mongo not passed")
	}

	log.Printf("get mongo for %s", address)
	session, err := mgo.Dial(address)
	if err != nil {
		log.Fatal("can't connect to mongo", err)
	}
	session.SetMode(mgo.Monotonic, true)

	creds := &mgo.Credential{Username: "root", Password: password}
	if password != "" {
		log.Println("login to mongo")
		if err = session.Login(creds); err != nil {
			log.Fatal("can't login to mongo", err)
		}
	}
	return &MongoServer{address: address, creds: creds, session: session, dbName: dbName}
}

//GetStores initilize collections and make indexes
func (m *MongoServer) GetStores() (rules RulesDAO) {

	rindexes := []mgo.Index{
		mgo.Index{Key: []string{"ts", "link"}},
		mgo.Index{Key: []string{"domain", "match_urls"}},
	}
	rules = RulesDAO{Collection: m.collection("rules", rindexes)}
	return rules
}

//collection makes collection with indexes
func (m *MongoServer) collection(collection string, indexes []mgo.Index) *mgo.Collection {
	log.Printf("create collection %s.%s", m.dbName, collection)
	coll := m.session.DB(m.dbName).C(collection)
	coll.Create(&mgo.CollectionInfo{ForceIdIndex: true})

	for _, index := range indexes {
		coll.EnsureIndex(index)
	}
	return coll
}
