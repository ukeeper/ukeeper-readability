package datastore

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRules(t *testing.T) {
	if _, ok := os.LookupEnv("ENABLE_MONGO_TESTS"); !ok {
		t.Skip("ENABLE_MONGO_TESTS env variable is not set")
	}
	server, err := New("mongodb://localhost:27017/", "test_ureadability", 0)
	require.NoError(t, err)
	assert.NotNil(t, server.client)
	rules := server.GetStores()
	assert.NotNil(t, rules)
	rule := Rule{
		Domain:  "example.com",
		Enabled: true,
	}

	// save a rule
	srule, err := rules.Save(context.Background(), rule)
	assert.NoError(t, err)
	assert.Equal(t, rule.Domain, srule.Domain)

	// get the rule we just saved
	grule, found := rules.Get(context.Background(), "https://"+rule.Domain)
	assert.True(t, found)
	assert.Equal(t, rule.Domain, grule.Domain)
	assert.Contains(t, rules.All(context.Background()), grule)

	// get the rule by ID (available after Get call)
	idrule, found := rules.GetByID(context.Background(), grule.ID)
	assert.True(t, found)
	assert.Equal(t, grule, idrule)

	// disable the rule
	err = rules.Disable(context.Background(), grule.ID)
	assert.NoError(t, err)
	assert.NotContains(t, rules.All(context.Background()), grule)

	// get the rule by ID, should be marked as disabled
	idrule, found = rules.GetByID(context.Background(), grule.ID)
	assert.True(t, found)
	assert.Equal(t, rule.Domain, grule.Domain)
	assert.False(t, idrule.Enabled)
	// same disabled rule still should appear in All call
	assert.Contains(t, rules.All(context.Background()), idrule)

	// get the disabled rule by domain, should not be found
	grule, found = rules.Get(context.Background(), "https://"+rule.Domain)
	assert.False(t, found)
	assert.Empty(t, grule.Domain)
}

func TestRulesCanceledContext(t *testing.T) {
	// we're not making requests to MongoDB, so it's ok to have no working connection
	server, err := New("mongodb://wrong", "", 0)
	require.NoError(t, err)
	assert.NotNil(t, server.client)
	rules := server.GetStores()
	assert.NotNil(t, rules)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// save a rule with canceled context
	rule := Rule{Domain: "example.com", Enabled: true}
	srule, err := rules.Save(ctx, rule)
	assert.Equal(t, rule, srule)
	assert.Error(t, err)

	// retrieve a rule, wrong rule
	grule, found := rules.Get(context.Background(), "http://user^:passwo^rd@foo.com/")
	assert.Empty(t, grule, "wrong URL")
	assert.False(t, found, "wrong URL")
	// retrieve a rule with canceled context
	grule, found = rules.Get(ctx, "")
	assert.Empty(t, grule, "canceled context")
	assert.False(t, found, "canceled context")
	assert.Empty(t, rules.All(ctx))
	assert.Error(t, rules.Disable(ctx, rule.ID))
	// get a rule by ID with canceled context
	grule, found = rules.GetByID(ctx, rule.ID)
	assert.Empty(t, grule)
	assert.False(t, found)
}
