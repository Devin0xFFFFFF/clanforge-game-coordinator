package main

import (
	"context"
	"time"

	"google.golang.org/appengine/datastore"
)

const (
	serverStateInitializing = 0
	serverStateActive       = 1
	serverStateEnding       = 2
	serverStateTerminating  = 3
)

type gameServer struct {
	UUID           string
	Address        string
	Port           int
	Region         string
	State          int
	CreationTime   time.Time
	CheckTime      time.Time
	PlayerCount    int
	MaxPlayerCount int
	Fill           float32
}

func queryServer(ctx context.Context, region string, queryNonEmpty bool) (*datastore.Key, gameServer, error) {
	var key *datastore.Key
	var server gameServer
	var err error

	var filter string

	if queryNonEmpty {
		filter = "Fill >"
	} else {
		filter = "PlayerCount ="
	}

	q := datastore.NewQuery("GameServer").Filter("Region =", region).Filter("State =", serverStateActive).Filter(filter, 0).Order("Fill").Limit(1).BatchSize(1)

	t := q.Run(ctx)
	key, err = t.Next(&server)

	return key, server, err
}
