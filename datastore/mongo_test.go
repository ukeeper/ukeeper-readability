package datastore

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-pkgz/testutils/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	mongoURI string // the one MongoDB container shared by all tests of the package
	dbSeq    atomic.Int64
)

// TestMain starts a single MongoDB container for the whole package. A container per test made the
// package take 20-30s on a healthy runner and went over the 60s limit on a slow one.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	mc, err := containers.NewMongoTestContainerE(context.Background(), 5)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't start mongo container: %v\n", err)
		return 1
	}
	defer mc.Close(context.Background()) //nolint:errcheck
	mongoURI = mc.URI
	return m.Run()
}

// testDBName returns a database name no other test uses, so tests sharing the container stay isolated
func testDBName() string {
	return fmt.Sprintf("test_ureadability_%d", dbSeq.Add(1))
}

func TestNew(t *testing.T) {
	t.Run("valid connection", func(t *testing.T) {
		server, err := New(mongoURI, "test_ureadability", 0)
		require.NoError(t, err)
		assert.NotNil(t, server)
		assert.NotNil(t, server.client)
		assert.Equal(t, "test_ureadability", server.dbName)
	})

	t.Run("with delay", func(t *testing.T) {
		server, err := New(mongoURI, "test_ureadability", 10*time.Millisecond)
		require.NoError(t, err)
		assert.NotNil(t, server)
	})

	t.Run("wrong connection string", func(t *testing.T) {
		server, err := New("wrong", "test_ureadability", 0)
		require.Error(t, err)
		assert.Nil(t, server)
	})

	t.Run("empty connection string", func(t *testing.T) {
		server, err := New("", "", 0)
		require.Error(t, err)
		assert.Nil(t, server)
	})
}

func TestGetStores(t *testing.T) {
	server, err := New(mongoURI, testDBName(), 0)
	require.NoError(t, err)

	stores := server.GetStores()
	assert.NotNil(t, stores.Rules)
	assert.NotNil(t, stores.Rules.Collection)
}
