package main

import (
	"context"
	"time"

	"google.golang.org/appengine/datastore"
)

const (
	mmStatusInQueue              = 0
	mmStatusJoinedMatch          = 1
	mmStatusMatchmakingCancelled = 2
	mmStatusMatchmakingFailed    = 3
)

type mmUser struct {
	UserID       string
	MMTok        string
	MMStatus     int
	CreationTime time.Time
	CheckTime    time.Time
	JoinTok      string
	ServerAddr   string
	ServerPort   int
}

func queryUser(ctx context.Context, filterKey string, filterArg string) (*datastore.Key, mmUser, error) {
	var key *datastore.Key
	var user mmUser
	var err error

	q := datastore.NewQuery("MMUser").Filter(filterKey, filterArg).Limit(1).BatchSize(1)
	t := q.Run(ctx)
	key, err = t.Next(&user)

	return key, user, err
}
