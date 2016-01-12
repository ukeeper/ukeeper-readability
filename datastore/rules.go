package datastore

//RulesDAO access methods to articles from/to mongo
import (
	"fmt"
	"log"
	"net/url"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

//Rules interface
type Rules interface {
	Get(rURL string) (Rule, bool)
	GetByID(id bson.ObjectId) (Rule, bool)
	Save(rule Rule) (Rule, error)
	Disable(id bson.ObjectId) error
	All() []Rule
}

//RulesDAO data-access obj for custom parsing rules, implements Rules
type RulesDAO struct {
	*mgo.Collection
}

//Rule record, entry in mongo
type Rule struct {
	ID        bson.ObjectId `json:"id" bson:"_id,omitempty"`
	Domain    string        `json:"domain"`
	MatchURLs []string      `json:"match_url,omitempty" bson:"match_urls,omitempty"`
	Content   string        `json:"content"`
	Author    string        `json:"author,omitempty" bson:"author,omitempty"`
	Ts        string        `json:"ts,omitempty" bson:"ts,omitempty"` //ts of original article
	Excludes  []string      `json:"excludes,omitempty" bson:"excludes,omitempty"`
	TestURLs  []string      `json:"test_urls,omitempty" bson:"test_urls"`
	User      string        `json:"user"`
	Enabled   bool          `json:"enabled"`
}

//Get rule by url. Checks if found in mongo, matching by domain
func (r RulesDAO) Get(rURL string) (Rule, bool) {
	u, err := url.Parse(rURL)
	if err != nil {
		log.Printf("failed to parse url=%s, error=%v", rURL, err)
		return Rule{}, false
	}

	var rules []Rule
	q := r.Collection.Find(bson.M{"domain": u.Host, "enabled": true})
	log.Printf("query %v", q)
	err = q.All(&rules)
	if err != nil && len(rules) == 0 {
		log.Printf("no custom rule for %s", rURL)
		return Rule{}, false
	}
	result := rules[0]
	log.Printf("found rule for %s = [%v]", rURL, result)
	return result, true
}

//GetByID returns record by id
func (r RulesDAO) GetByID(id bson.ObjectId) (Rule, bool) {
	var rule Rule
	err := r.Collection.Find(bson.M{"_id": id}).One(&rule)
	return rule, err == nil
}

//Save upsert rule and returns one with ID for insterted one only
func (r RulesDAO) Save(rule Rule) (Rule, error) {
	ch, err := r.Collection.Upsert(bson.M{"domain": rule.Domain}, rule)
	if err != nil {
		log.Printf("failed to save, error=%v, article=%v", err, rule)
		return rule, err
	}
	if ch.UpsertedId != nil {
		rule.ID = ch.UpsertedId.(bson.ObjectId)
	}
	return rule, err
}

//Disable marks enabled=false, by id
func (r RulesDAO) Disable(id bson.ObjectId) error {
	return r.Collection.Update(bson.M{"_id": id}, bson.M{"$set": bson.M{"enabled": false}})
}

//All returns list of all rules, both enabled and disabled
func (r RulesDAO) All() []Rule {
	result := []Rule{}
	if err := r.Collection.Find(bson.M{}).All(&result); err != nil {
		return []Rule{}
	}
	return result
}

func (s Rule) String() string {
	return fmt.Sprintf("{id=%s, domain=%s, content=%s, enabled=%v}", s.ID.Hex(), s.Domain, s.Content, s.Enabled)
}
