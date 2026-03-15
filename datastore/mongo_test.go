package datastore

import (
	"context"
	"testing"
	"time"

	"github.com/go-pkgz/testutils/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	mc := containers.NewMongoTestContainer(context.Background(), t, 5)
	defer mc.Close(context.Background()) //nolint:errcheck

	t.Run("valid connection", func(t *testing.T) {
		server, err := New(mc.URI, "test_ureadability", 0)
		require.NoError(t, err)
		assert.NotNil(t, server)
		assert.NotNil(t, server.client)
		assert.Equal(t, "test_ureadability", server.dbName)
	})

	t.Run("with delay", func(t *testing.T) {
		server, err := New(mc.URI, "test_ureadability", 10*time.Millisecond)
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
	mc := containers.NewMongoTestContainer(context.Background(), t, 5)
	defer mc.Close(context.Background()) //nolint:errcheck

	server, err := New(mc.URI, "test_ureadability", 0)
	require.NoError(t, err)

	stores := server.GetStores()
	assert.NotNil(t, stores.Rules)
	assert.NotNil(t, stores.Rules.Collection)
}
