package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
)

const (
	userRecordExpiryTime = 1
	joinRecordExpiryTime = 1
)

type matchmakerStats struct {
	Timestamp    time.Time
	TotalUsers   int
	TotalJoinsNA int
	TotalJoinsEU int
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	log.Infof(ctx, "[Stats] Running User Stats Collection...")

	collectMatchmakerStats(ctx)

	log.Infof(ctx, "[Stats] Running Server Stats Collection...")

	collectServerStats(ctx)

	log.Infof(ctx, "[Stats] Running User Expiration...")

	expireUsers(ctx)

	log.Infof(ctx, "[Stats] Running Join Expiration...")

	expireJoins(ctx)
}

func collectMatchmakerStats(ctx context.Context) {
	stats := matchmakerStats{Timestamp: time.Now()}

	userQuery := datastore.NewQuery("MMUser")
	userCount, err := userQuery.Count(ctx)

	if err != nil {
		log.Errorf(ctx, "[Stats] %v", err.Error())
	}

	stats.TotalUsers = userCount

	joinsNAQuery := datastore.NewQuery("JoinRecord").Filter("Region =", naRegionName)
	joinsNACount, err := joinsNAQuery.Count(ctx)

	if err != nil {
		log.Errorf(ctx, "[Stats] %v", err.Error())
	}

	joinsEUQuery := datastore.NewQuery("JoinRecord").Filter("Region =", euRegionName)
	joinsEUCount, err := joinsEUQuery.Count(ctx)

	if err != nil {
		log.Errorf(ctx, "[Stats] %v", err.Error())
	}

	stats.TotalJoinsNA = joinsNACount
	stats.TotalJoinsEU = joinsEUCount

	buffer := &bytes.Buffer{}
	w := csv.NewWriter(buffer)
	record := make([]string, 4)

	record[0] = "Timestamp"
	record[1] = "TotalUsers"
	record[2] = "TotalJoinsNA"
	record[3] = "TotalJoinsEU"

	w.Write(record)

	record[0] = fmt.Sprint(stats.Timestamp.Unix())
	record[1] = fmt.Sprint(stats.TotalUsers)
	record[2] = fmt.Sprint(stats.TotalJoinsNA)
	record[3] = fmt.Sprint(stats.TotalJoinsEU)

	w.Write(record)

	w.Flush()

	fileName := fmt.Sprintf("stats/matchmaker/%v.csv", time.Now().Format("20060102150405"))

	err = storeCSV(ctx, fileName, buffer.Bytes())

	if err != nil {
		log.Errorf(ctx, "[Stats] %v", err.Error())
	}
}

func collectServerStats(ctx context.Context) {
	var stats serverStats
	var err error

	q := datastore.NewQuery("ServerStats")

	buffer := &bytes.Buffer{}
	w := csv.NewWriter(buffer)
	keys := []*datastore.Key{}
	record := make([]string, 5)

	record[0] = "Region"
	record[1] = "Timestamp"
	record[2] = "TotalServers"
	record[3] = "TotalCurrentPlayers"
	record[4] = "TotalMaxPlayers"

	w.Write(record)

	for t := q.Run(ctx); ; {
		key, err := t.Next(&stats)

		if err != nil {
			if err == datastore.Done {
				break
			} else {
				log.Errorf(ctx, "[Stats] %v", err.Error())
				continue
			}
		}

		keys = append(keys, key)

		record[0] = stats.Region
		record[1] = fmt.Sprint(stats.Timestamp.Unix())
		record[2] = fmt.Sprint(stats.TotalServers)
		record[3] = fmt.Sprint(stats.TotalCurrentPlayers)
		record[4] = fmt.Sprint(stats.TotalMaxPlayers)

		w.Write(record)
	}

	w.Flush()

	fileName := fmt.Sprintf("stats/servers/%v.csv", time.Now().Format("20060102150405"))

	err = storeCSV(ctx, fileName, buffer.Bytes())

	if err != nil {
		log.Errorf(ctx, "[Stats] %v", err.Error())
	}

	err = removeFromDatastore(ctx, keys)

	if err != nil {
		log.Errorf(ctx, "[Stats] %v", err.Error())
		return
	}

	log.Infof(ctx, "[Stats] Removed %v ServerStats records.", len(keys))
}

func expireUsers(ctx context.Context) {
	userCheckTime := time.Now().Add(-userRecordExpiryTime * time.Hour)
	userQuery := datastore.NewQuery("MMUser").Filter("CheckTime <", userCheckTime)

	var mmUser mmUser

	userKeys := []*datastore.Key{}

	for t := userQuery.Run(ctx); ; {
		key, err := t.Next(&mmUser)

		if err != nil {
			if err == datastore.Done {
				break
			} else {
				log.Errorf(ctx, "[Stats] %v", err.Error())
				continue
			}
		}

		userKeys = append(userKeys, key)
	}

	err := removeFromDatastore(ctx, userKeys)

	if err != nil {
		log.Errorf(ctx, "[Stats] %v", err.Error())
	} else {
		log.Infof(ctx, "[Stats] Removed %v User records.", len(userKeys))
	}
}

func expireJoins(ctx context.Context) {
	joinCheckTime := time.Now().Add(-joinRecordExpiryTime * time.Minute)
	joinQuery := datastore.NewQuery("JoinRecord").Filter("CreationTime <", joinCheckTime)

	var joinRecord joinRecord

	joinKeys := []*datastore.Key{}

	for t := joinQuery.Run(ctx); ; {
		key, err := t.Next(&joinRecord)

		if err != nil {
			if err == datastore.Done {
				break
			} else {
				log.Errorf(ctx, "[Stats] %v", err.Error())
				continue
			}
		}

		joinKeys = append(joinKeys, key)
	}

	err := removeFromDatastore(ctx, joinKeys)

	if err != nil {
		log.Errorf(ctx, "[Stats] %v", err.Error())
	} else {
		log.Infof(ctx, "[Stats] Removed %v Join records.", len(joinKeys))
	}
}
