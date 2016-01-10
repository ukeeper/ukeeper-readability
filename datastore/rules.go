package datastore

//RulesDAO access methods to articles from/to mongo
import (
	"log"
	"net/url"
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

//Rules interface
type Rules interface {
	Get(rURL string) (Rule, bool)
}

//RulesDAO data-access obj for custom parsing rules, implements Rules
type RulesDAO struct {
	*mgo.Collection
}

//Rule record, entry in mongo
type Rule struct {
	ID        bson.ObjectId `json:"id" bson:"_id,omitempty"`
	Domain    string        `json:"domain"`
	MatchURLs []string      `json:"match_url" bson:"match_urls,omitempty"`
	Content   string        `json:"content"`
	Author    string        `json:"author" bson:"author,omitempty"`
	Ts        time.Time     `json:"ts" bson:"ts,omitempty"` //ts of original article
	Excludes  []string      `json:"excludes" bson:"excludes,omitempty"`
	TestURLs  []string      `json:"test_urls" bson:"test_urls"`
	User      string        `json:"user"`
	Enabled   bool          `json:"enabled"`
}

func (r RulesDAO) get(rURL string) (Rule, bool) {
	u, err := url.Parse(rURL)
	if err != nil {
		return Rule{}, false
	}

	var rules []Rule
	r.Collection.Find(bson.M{"domain": u.Host, "enable": true}).All(&rules)
	if len(rules) == 0 {
		return Rule{}, false
	}
	result := rules[0]
	log.Printf("found rule for %s = [%v]", rURL, result)
	return result, true
}
