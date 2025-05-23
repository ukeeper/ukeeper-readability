package datastore

import (
	"context"
	"math/rand/v2"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyz"

func TestRules(t *testing.T) {
	if _, ok := os.LookupEnv("ENABLE_MONGO_TESTS"); !ok {
		t.Skip("ENABLE_MONGO_TESTS env variable is not set")
	}
	server, err := New("mongodb://localhost:27017/", "test_ureadability", 0)
	require.NoError(t, err)
	assert.NotNil(t, server.client)
	stores := server.GetStores()
	assert.NotNil(t, stores)
	rules := stores.Rules
	assert.NotNil(t, rules)
	rule := Rule{
		Domain:  randStringBytesRmndr(42) + ".com",
		Enabled: true,
	}

	// save a rule
	srule, err := rules.Save(context.Background(), rule)
	require.NoError(t, err)
	assert.Equal(t, rule.Domain, srule.Domain)
	ruleID := srule.ID

	// get the rule we just saved by domain
	grule, found := rules.Get(context.Background(), "https://"+rule.Domain)
	assert.True(t, found)
	assert.Equal(t, rule.Domain, grule.Domain)
	assert.Equal(t, ruleID, grule.ID)
	assert.Contains(t, rules.All(context.Background()), grule)

	// get the rule by ID
	idrule, found := rules.GetByID(context.Background(), ruleID)
	assert.True(t, found)
	assert.Equal(t, grule, idrule)

	// disable the rule
	err = rules.Disable(context.Background(), grule.ID)
	require.NoError(t, err)
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

	// save a rule once more, should result in the same ID
	updatedRule, err := rules.Save(context.Background(), rule)
	require.NoError(t, err)
	assert.Equal(t, rule.Domain, updatedRule.Domain)
	assert.Equal(t, ruleID, updatedRule.ID)
}

func TestRulesCanceledContext(t *testing.T) {
	// we're not making requests to MongoDB, so it's ok to have no working connection
	server, err := New("mongodb://wrong", "", 0)
	require.NoError(t, err)
	assert.NotNil(t, server.client)
	stores := server.GetStores()
	assert.NotNil(t, stores)
	rules := stores.Rules
	assert.NotNil(t, rules)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// save a rule with canceled context
	rule := Rule{Domain: "example.com", Enabled: true}
	srule, err := rules.Save(ctx, rule)
	assert.Equal(t, rule, srule)
	require.Error(t, err)

	// retrieve a rule, wrong rule
	grule, found := rules.Get(context.Background(), "http://user^:passwo^rd@foo.com/")
	assert.Empty(t, grule, "wrong URL")
	assert.False(t, found, "wrong URL")
	// retrieve a rule with canceled context
	grule, found = rules.Get(ctx, "")
	assert.Empty(t, grule, "canceled context")
	assert.False(t, found, "canceled context")
	assert.Empty(t, rules.All(ctx))
	require.Error(t, rules.Disable(ctx, rule.ID))
	// get a rule by ID with canceled context
	grule, found = rules.GetByID(ctx, rule.ID)
	assert.Empty(t, grule)
	assert.False(t, found)
}

// thanks to https://stackoverflow.com/a/31832326/961092
func randStringBytesRmndr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int64()%int64(len(letterBytes))]
	}
	return string(b)
}
