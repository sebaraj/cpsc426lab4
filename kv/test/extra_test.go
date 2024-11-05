package kvtest

import (
	// "sync"
	"testing"
	"time"

	"cs426.yale.edu/lab4/kv"
	// "cs426.yale.edu/lab4/kv/proto"
	// "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Like previous labs, you must write some tests on your own.
// Add your test cases in this file and submit them as extra_test.go.
// You must add at least 5 test cases, though you can add as many as you like.
//
// You can use any type of test already used in this lab: server
// tests, client tests, or integration tests.
//
// You can also write unit tests of any utility functions you have in utils.go
//
// Tests are run from an external package, so you are testing the public API
// only. You can make methods public (e.g. utils) by making them Capitalized.

func TestYourTestServerWrongServerForShard(t *testing.T) {
	// 2 nodes, 2 shards, 1 shard each; get the shard/node for key, send quest to other key
	// tests scenario where node possess some shard (unlike other delete tests), but the wrong shard
	setup := MakeTestSetup(kv.ShardMapState{
		NumShards: 2,
		Nodes:     makeNodeInfos(2),
		ShardsToNodes: map[int][]string{
			1: {"n1"},
			2: {"n2"},
		},
	})

	err := setup.NodeSet("n1", "abc", "123", 10*time.Second)
	// if this fails, abc should be on n2
	var wrongNode string
	if err != nil {
		err = setup.NodeSet("n2", "abc", "123", 10*time.Second)
		wrongNode = "n1"
	} else {
		wrongNode = "n2"
	}
	assert.Nil(t, err)
	val, wasFound, err := setup.NodeGet(wrongNode, "abc")
	assert.Equal(t, val, "")
	assert.False(t, wasFound)
	assert.NotNil(t, err)
	e, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.NotFound, e.Code())

	setup.Shutdown()
}

func TestYourTestServerTTLOverwrite(t *testing.T) {
	// checks that TTL is overwritten, not added
	setup := MakeTestSetup(kv.ShardMapState{
		NumShards: 2,
		Nodes:     makeNodeInfos(2),
		ShardsToNodes: map[int][]string{
			1: {"n1"},
			2: {"n2"},
		},
	})
	var correctNode string
	err := setup.NodeSet("n1", "abc", "123", 10*time.Second)
	if err != nil {
		err = setup.NodeSet("n2", "abc", "123", 10*time.Second)
		correctNode = "n2"
	} else {
		correctNode = "n1"
	}
	assert.Nil(t, err)
	valOriginal, wasFoundOriginal, err := setup.NodeGet(correctNode, "abc")
	assert.Equal(t, valOriginal, "123")
	assert.True(t, wasFoundOriginal)
	assert.Nil(t, err)
	err = setup.NodeSet(correctNode, "abc", "123", 100*time.Millisecond)
	assert.Nil(t, err)
	time.Sleep(200 * time.Millisecond)
	val, wasFound, err := setup.NodeGet(correctNode, "abc")
	assert.Equal(t, val, "")
	assert.False(t, wasFound)
	assert.Nil(t, err) // should be nil on ttl expiration

	setup.Shutdown()
}

func TestYourTestClientEmptyKey(t *testing.T) {
	// Attempting to Get/Set/Delete an empty key; should return an error.
	// Non-trivial test, b/c if the request.Key == "" check is after the logrus.WithFields call, an empty key will seg fault
	setup := MakeTestSetupWithoutServers(MakeBasicOneShard())

	// Attempt to Get an empty key
	_, _, err := setup.Get("")
	assert.NotNil(t, err, "Expected error when getting an empty key")
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "Expected InvalidArgument error code")

	// Attempt to Set an empty key
	err = setup.Set("", "value", 1*time.Second)
	assert.NotNil(t, err, "Expected error when setting an empty key")
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "Expected InvalidArgument error code")
	//
	// // Attempt to Delete an empty key
	err = setup.Delete("")
	assert.NotNil(t, err, "Expected error when deleting an empty key")
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "Expected InvalidArgument error code")
}

func TestYourTestClientKeyExpiration(t *testing.T) {
	// Tests that keys expire correctly after their TTL has passed
	setup := MakeTestSetupWithoutServers(MakeBasicOneShard())
	setup.clientPool.OverrideSetResponse("n1")

	key := "tempKey"
	value := "tempValue"
	ttl := 500 * time.Millisecond // Short TTL of 500ms

	err := setup.Set(key, value, ttl)
	assert.Nil(t, err, "Set should succeed")
	setup.clientPool.OverrideGetResponse("n1", value, true)
	val, wasFound, err := setup.Get(key)
	assert.Nil(t, err)
	assert.True(t, wasFound)
	assert.Equal(t, value, val, "Value should match before expiration")
	time.Sleep(ttl + 100*time.Millisecond)
	setup.clientPool.OverrideGetResponse("n1", "", false)

	val, wasFound, err = setup.Get(key)
	assert.Nil(t, err)
	assert.False(t, wasFound, "Key should not be found after expiration")
	assert.Equal(t, "", val, "Value should be empty after expiration")
}

func TestYourTestServerSetNegativeTTL(t *testing.T) {
	// Tests that setting a key with a negative TTL returns an error

	setup := MakeTestSetup(MakeBasicOneShard())

	err := setup.NodeSet("n1", "negativeTTLKey", "value", -10*time.Second)
	assert.NotNil(t, err, "Expected error when setting a key with negative TTL")
	assertErrorWithCode(t, err, codes.InvalidArgument)

	_, wasFound, err := setup.NodeGet("n1", "negativeTTLKey")
	assert.Nil(t, err, "Get operation should not return an error")
	assert.False(t, wasFound, "Key should not be found after setting with negative TTL")

	setup.Shutdown()
}
