package datastore

import (
	"context"
	"math/rand/v2"
	"testing"

	"github.com/go-pkgz/testutils/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyz"

func TestRulesSave(t *testing.T) {
	rules := setupRules(t)

	t.Run("save new rule", func(t *testing.T) {
		rule := Rule{Domain: randDomain(), Content: "article p", Enabled: true}
		saved, err := rules.Save(context.Background(), rule)
		require.NoError(t, err)
		assert.Equal(t, rule.Domain, saved.Domain)
		assert.Equal(t, rule.Content, saved.Content)
		assert.True(t, saved.Enabled)
		assert.NotEqual(t, bson.NilObjectID, saved.ID)
	})

	t.Run("upsert same domain preserves id", func(t *testing.T) {
		domain := randDomain()
		rule := Rule{Domain: domain, Content: "original", Enabled: true}
		saved, err := rules.Save(context.Background(), rule)
		require.NoError(t, err)
		origID := saved.ID

		updated := Rule{Domain: domain, Content: "updated", Enabled: true}
		saved, err = rules.Save(context.Background(), updated)
		require.NoError(t, err)
		assert.Equal(t, origID, saved.ID)
		assert.Equal(t, "updated", saved.Content)
	})

	t.Run("save with all fields", func(t *testing.T) {
		rule := Rule{
			Domain:    randDomain(),
			Content:   ".post-content",
			Author:    "test-author",
			MatchURLs: []string{"/blog/*"},
			Excludes:  []string{".sidebar"},
			TestURLs:  []string{"https://example.com/test"},
			Enabled:   true,
		}
		saved, err := rules.Save(context.Background(), rule)
		require.NoError(t, err)
		assert.Equal(t, rule.Domain, saved.Domain)
		assert.Equal(t, rule.Author, saved.Author)
		assert.Equal(t, rule.MatchURLs, saved.MatchURLs)
		assert.Equal(t, rule.Excludes, saved.Excludes)
		assert.Equal(t, rule.TestURLs, saved.TestURLs)
	})

	t.Run("save with canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		rule := Rule{Domain: "example.com", Enabled: true}
		_, err := rules.Save(ctx, rule)
		require.Error(t, err)
	})
}

func TestRulesGet(t *testing.T) {
	rules := setupRules(t)

	t.Run("get existing enabled rule by url", func(t *testing.T) {
		domain := randDomain()
		rule := Rule{Domain: domain, Content: "article", Enabled: true}
		_, err := rules.Save(context.Background(), rule)
		require.NoError(t, err)

		found, ok := rules.Get(context.Background(), "https://"+domain+"/some/path")
		assert.True(t, ok)
		assert.Equal(t, domain, found.Domain)
		assert.Equal(t, "article", found.Content)
	})

	t.Run("disabled rule not found", func(t *testing.T) {
		domain := randDomain()
		rule := Rule{Domain: domain, Content: "article", Enabled: true}
		saved, err := rules.Save(context.Background(), rule)
		require.NoError(t, err)
		err = rules.Disable(context.Background(), saved.ID)
		require.NoError(t, err)

		_, ok := rules.Get(context.Background(), "https://"+domain+"/page")
		assert.False(t, ok)
	})

	t.Run("non-existing domain", func(t *testing.T) {
		found, ok := rules.Get(context.Background(), "https://nonexistent-domain-xyz.com/page")
		assert.False(t, ok)
		assert.Empty(t, found.Domain)
	})

	t.Run("invalid url", func(t *testing.T) {
		found, ok := rules.Get(context.Background(), "http://user^:passwo^rd@foo.com/")
		assert.False(t, ok)
		assert.Empty(t, found)
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, ok := rules.Get(ctx, "https://example.com")
		assert.False(t, ok)
	})
}

func TestRulesGetByID(t *testing.T) {
	rules := setupRules(t)

	t.Run("existing rule", func(t *testing.T) {
		rule := Rule{Domain: randDomain(), Content: "article", Enabled: true}
		saved, err := rules.Save(context.Background(), rule)
		require.NoError(t, err)

		found, ok := rules.GetByID(context.Background(), saved.ID)
		assert.True(t, ok)
		assert.Equal(t, saved.ID, found.ID)
		assert.Equal(t, rule.Domain, found.Domain)
	})

	t.Run("non-existing id", func(t *testing.T) {
		_, ok := rules.GetByID(context.Background(), bson.NewObjectID())
		assert.False(t, ok)
	})

	t.Run("nil object id", func(t *testing.T) {
		_, ok := rules.GetByID(context.Background(), bson.NilObjectID)
		assert.False(t, ok)
	})
}

func TestRulesDisable(t *testing.T) {
	rules := setupRules(t)

	t.Run("disable existing rule", func(t *testing.T) {
		rule := Rule{Domain: randDomain(), Enabled: true}
		saved, err := rules.Save(context.Background(), rule)
		require.NoError(t, err)
		assert.True(t, saved.Enabled)

		err = rules.Disable(context.Background(), saved.ID)
		require.NoError(t, err)

		found, ok := rules.GetByID(context.Background(), saved.ID)
		assert.True(t, ok)
		assert.False(t, found.Enabled)
	})

	t.Run("disable non-existing id does not error", func(t *testing.T) {
		err := rules.Disable(context.Background(), bson.NewObjectID())
		require.NoError(t, err) // mongo UpdateOne with no match is not an error
	})
}

func TestRulesAll(t *testing.T) {
	rules := setupRules(t)

	t.Run("returns all rules including disabled", func(t *testing.T) {
		domain1 := randDomain()
		domain2 := randDomain()
		_, err := rules.Save(context.Background(), Rule{Domain: domain1, Content: "a", Enabled: true})
		require.NoError(t, err)
		saved2, err := rules.Save(context.Background(), Rule{Domain: domain2, Content: "b", Enabled: true})
		require.NoError(t, err)
		err = rules.Disable(context.Background(), saved2.ID)
		require.NoError(t, err)

		all := rules.All(context.Background())
		assert.GreaterOrEqual(t, len(all), 2)

		var foundEnabled, foundDisabled bool
		for _, r := range all {
			if r.Domain == domain1 {
				foundEnabled = true
				assert.True(t, r.Enabled)
			}
			if r.Domain == domain2 {
				foundDisabled = true
				assert.False(t, r.Enabled)
			}
		}
		assert.True(t, foundEnabled, "enabled rule should be in All()")
		assert.True(t, foundDisabled, "disabled rule should be in All()")
	})

	t.Run("canceled context returns empty", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		all := rules.All(ctx)
		assert.Empty(t, all)
	})
}

func TestRuleString(t *testing.T) {
	rule := Rule{
		ID:      bson.NewObjectID(),
		Domain:  "example.com",
		Content: ".article",
		Enabled: true,
	}
	s := rule.String()
	assert.Contains(t, s, "example.com")
	assert.Contains(t, s, ".article")
	assert.Contains(t, s, "enabled=true")
}

func setupRules(t *testing.T) RulesDAO {
	t.Helper()
	mc := containers.NewMongoTestContainer(context.Background(), t, 5)
	t.Cleanup(func() { mc.Close(context.Background()) }) //nolint:errcheck

	server, err := New(mc.URI, "test_ureadability", 0)
	require.NoError(t, err)
	stores := server.GetStores()
	return stores.Rules
}

func randDomain() string {
	return randStringBytesRmndr(20) + ".com"
}

func randStringBytesRmndr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int64()%int64(len(letterBytes))]
	}
	return string(b)
}
