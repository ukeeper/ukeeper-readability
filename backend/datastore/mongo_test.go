package datastore

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMongoCreation(t *testing.T) {
	if _, ok := os.LookupEnv("ENABLE_MONGO_TESTS"); !ok {
		t.Skip("ENABLE_MONGO_TESTS env variable is not set")
	}
	// wrong credentials, so that GetStores will fail with warning
	server, err := New("mongodb://wrong:wrong@localhost:27017/", "test_ureadability", 0)
	require.NoError(t, err)
	assert.NotNil(t, server)
	assert.NotNil(t, server.client)
	assert.Equal(t, "test_ureadability", server.dbName)
	assert.NotNil(t, server.GetStores())
}

func TestWrongConnectionString(t *testing.T) {
	server, err := New("wrong", "test_ureadability", time.Millisecond*100)
	assert.Error(t, err)
	assert.Nil(t, server)
	server, err = New("", "", time.Millisecond*100)
	assert.Error(t, err)
	assert.Nil(t, server)
}
