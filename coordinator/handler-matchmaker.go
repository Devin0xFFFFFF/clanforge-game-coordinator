package main

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gofrs/uuid"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
)

const (
	mmLastServerKey           = "Matchmaker-LastServer"
	userNotFoundRetryAttempts = 3
	noServersRetryAttempts    = 5
)

func joinMatchHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	var mmtok string
	var region string

	mmtok = r.FormValue("mmtok")
	region = r.FormValue("region")

	attemptsHeader := r.Header.Get("X-AppEngine-TaskRetryCount")
	attempts, err := strconv.Atoi(attemptsHeader)

	if err != nil {
		log.Errorf(ctx, "[JoinMatch] %v", err.Error())
		return // Return 200 for the request to disregard it
	}

	log.Debugf(ctx, "[JoinMatch] Handling join request for %v in region %v (attempt %v)", mmtok, region, attemptsHeader)

	// Get User

	userKey, mmUser, uErr := queryUser(ctx, "MMTok =", mmtok)
	foundUser := userKey != nil

	if !foundUser {
		if uErr == datastore.Done { // Possible if dequeued prior to join attempt
			if attempts >= userNotFoundRetryAttempts {
				log.Errorf(ctx, "[JoinMatch] Out of attempts to find user with token: %v", mmtok)
				return // Return 200 for the request to disregard it
			}
			log.Errorf(ctx, "[JoinMatch] Could not find user with token: %v", mmtok)
			http.Error(w, "Token Not Found.", http.StatusNotFound)
			return
		} else if uErr != nil {
			log.Errorf(ctx, "[JoinMatch] %v", uErr.Error())
			http.Error(w, "Unexpected error.", http.StatusInternalServerError)
			return
		}
	}

	if mmUser.MMStatus != mmStatusInQueue { // Already out of queue
		return // Return 200 for the request to disregard it
	}

	if attempts > noServersRetryAttempts {
		mmUser.MMStatus = mmStatusMatchmakingFailed

		_, err = datastore.Put(ctx, userKey, &mmUser)
		if err != nil {
			log.Errorf(ctx, "[JoinMatch] %v", err.Error())
			http.Error(w, "Unexpected error.", http.StatusInternalServerError)
			return
		}

		return
	}

	//Get Server

	var sk *datastore.Key
	var serverKey datastore.Key
	var server gameServer
	var sErr error
	var foundKey bool

	serverItem, err := memcache.Get(ctx, mmLastServerKey+region)

	if err == nil { // Found stored server key
		encodedKey := serverItem.Value
		err = serverKey.GobDecode(encodedKey)
		if err != nil {
			log.Errorf(ctx, "[JoinMatch] %v", err.Error())
		} else {
			foundKey = true
		}
	} else if err != memcache.ErrCacheMiss {
		log.Errorf(ctx, "[JoinMatch] %v", err.Error())
	}

	if foundKey { // If previous server key was stored in memcache, retrieve it
		err := datastore.Get(ctx, &serverKey, &server)
		if err != nil {
			memcache.Delete(ctx, mmLastServerKey+region) // Remove key as server is not found, next pass will find new server
			log.Errorf(ctx, "[JoinMatch] %v", err.Error())
			http.Error(w, "Unexpected error.", http.StatusInternalServerError)
			return
		} else if server.State != serverStateActive {
			memcache.Delete(ctx, mmLastServerKey+region) // Remove key as server is in invalid state, next pass will find new server
			log.Errorf(ctx, "[JoinMatch] Cached Server in Invalid State.")
			http.Error(w, "Server in Invalid State.", http.StatusInternalServerError)
			return
		} else if server.PlayerCount >= server.MaxPlayerCount {
			memcache.Delete(ctx, mmLastServerKey+region) // Remove key as server is full, next pass will find new server
			log.Errorf(ctx, "[JoinMatch] Cached Server Full.")
			http.Error(w, "Cached Server Full.", http.StatusInternalServerError)
			return
		}
	} else { // Else query the datastore for a server to join
		sk, server, sErr = queryServer(ctx, region, true)

		if sErr == datastore.Done { // No available non-empty servers
			sk, server, sErr = queryServer(ctx, region, false)
			if sErr == datastore.Done { // No available servers
				log.Errorf(ctx, "[JoinMatch] No Available Servers")
				http.Error(w, "No Available Servers", http.StatusServiceUnavailable)
				return
			} else if sErr != nil {
				log.Errorf(ctx, "[JoinMatch] %v", sErr.Error())
				http.Error(w, "Unexpected error.", http.StatusInternalServerError)
				return
			} else if server.PlayerCount >= server.MaxPlayerCount {
				log.Errorf(ctx, "[JoinMatch] Retrieved Server Full.")
				http.Error(w, "Retrieved Server Full.", http.StatusInternalServerError)
				return
			}
		} else if sErr != nil {
			log.Errorf(ctx, "[JoinMatch] %v", sErr.Error())
			http.Error(w, "Unexpected error.", http.StatusInternalServerError)
			return
		} else if server.PlayerCount >= server.MaxPlayerCount {
			log.Errorf(ctx, "[JoinMatch] Retrieved Server Full.")
			http.Error(w, "Retrieved Server Full.", http.StatusInternalServerError)
			return
		}

		serverKey = *sk

		// Store the server key as the new last server

		newEncodedKey, err := serverKey.GobEncode()

		if err != nil {
			log.Errorf(ctx, "[JoinMatch] %v", sErr.Error())
		} else {
			serverItem := &memcache.Item{
				Key:   mmLastServerKey + region,
				Value: newEncodedKey,
			}

			err = memcache.Set(ctx, serverItem)

			if err != nil {
				log.Errorf(ctx, "[JoinMatch] %v", sErr.Error())
			}
		}
	}

	server.PlayerCount++

	if server.PlayerCount >= server.MaxPlayerCount {
		memcache.Delete(ctx, mmLastServerKey+region)
	}

	_, err = datastore.Put(ctx, &serverKey, &server)
	if err != nil {
		log.Errorf(ctx, "[JoinMatch] %v", err.Error())
		http.Error(w, "[JoinMatch] Unexpected error.", http.StatusInternalServerError)
		return
	}

	joinTok := uuid.Must(uuid.NewV4()).String()

	// Store join record to notify server of joining player

	join := joinRecord{
		UserID:       mmUser.UserID,
		ServerID:     server.UUID,
		Region:       region,
		JoinToken:    joinTok,
		CreationTime: time.Now(),
		Checked:      false,
	}

	_, err = datastore.Put(ctx, datastore.NewIncompleteKey(ctx, "JoinRecord", nil), &join)
	if err != nil {
		log.Errorf(ctx, "[JoinMatch] %v", err.Error())
		http.Error(w, "[JoinMatch] Unexpected error.", http.StatusInternalServerError)
		return
	}

	// Update player state

	mmUser.MMStatus = mmStatusJoinedMatch
	mmUser.JoinTok = joinTok
	mmUser.ServerAddr = server.Address
	mmUser.ServerPort = server.Port

	_, err = datastore.Put(ctx, userKey, &mmUser)
	if err != nil {
		log.Errorf(ctx, "[JoinMatch] %v", err.Error())
		http.Error(w, "Unexpected error.", http.StatusInternalServerError)
		return
	}

	log.Infof(ctx, "[JoinMatch] User %v (%v) joined server %v (%v, %v)", mmtok, mmUser.UserID, server.UUID, server.Address, server.Port)

	fmt.Fprintf(w, "%v (%v) joined %v (%v, %v)", mmtok, mmUser.UserID, server.UUID, server.Address, server.Port)
}
