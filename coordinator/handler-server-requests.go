package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
)

type joinInfo struct {
	UserID    string `json:"UserID"`
	JoinToken string `json:"JoinToken"`
}

type joinReport struct {
	JoinInfo []joinInfo `json:"JoinInfo"`
}

func heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	if r.Method != "GET" {
		log.Errorf(ctx, "[Heartbeat] Invalid request method %v", r.Method)
		http.Error(w, "Invalid Request.", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()

	serverID := q.Get("ServerID")
	serverState := q.Get("ServerState")
	playerCount := q.Get("PlayerCount")
	maxPlayerCount := q.Get("MaxPlayerCount")

	players, err := strconv.ParseInt(playerCount, 10, 32)

	if err != nil {
		log.Errorf(ctx, "[Heartbeat] Invalid request args")
		http.Error(w, "Invalid Request.", http.StatusBadRequest)
		return
	}

	state, err := strconv.ParseInt(serverState, 10, 32)

	if err != nil {
		log.Errorf(ctx, "[Heartbeat] Invalid request args")
		http.Error(w, "Invalid Request.", http.StatusBadRequest)
		return
	}

	maxPlayers, err := strconv.ParseInt(maxPlayerCount, 10, 32)

	if err != nil {
		log.Errorf(ctx, "[Heartbeat] Invalid request args")
		http.Error(w, "Invalid Request.", http.StatusBadRequest)
		return
	}

	var server gameServer

	query := datastore.NewQuery("GameServer").Filter("UUID =", serverID)
	t := query.Run(ctx)
	serverKey, err := t.Next(&server)

	if err == datastore.Done { // No available non-empty servers
		log.Errorf(ctx, "[Heartbeat] Server not Found: "+serverID)
		http.Error(w, "Server not Found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Errorf(ctx, "[Heartbeat] %v", err.Error())
		http.Error(w, "Unexpected error.", http.StatusInternalServerError)
		return
	}

	server.State = int(state)
	server.CheckTime = time.Now()
	server.PlayerCount = int(players)
	server.MaxPlayerCount = int(maxPlayers)
	server.Fill = float32(server.PlayerCount) / float32(server.MaxPlayerCount)

	_, err = datastore.Put(ctx, serverKey, &server)
	if err != nil {
		log.Errorf(ctx, "[Heartbeat] %v", err.Error())
		http.Error(w, "Unexpected error.", http.StatusInternalServerError)
		return
	}

	// Get all unprocessed joins and forward then to the server

	joinQuery := datastore.NewQuery("JoinRecord").Filter("ServerID =", serverID).Filter("Checked = ", false)

	var join joinRecord
	joins := []joinInfo{}
	i := 0

	for t := joinQuery.Run(ctx); ; {
		key, err := t.Next(&join)
		if err == datastore.Done {
			break
		}
		if err != nil {
			log.Errorf(ctx, "[Heartbeat] %v", err.Error())
			return
		}

		joins = append(joins, joinInfo{
			UserID:    join.UserID,
			JoinToken: join.JoinToken,
		})

		join.Checked = true

		_, err = datastore.Put(ctx, key, &join)
		if err != nil {
			log.Errorf(ctx, "[Heartbeat] %v", err.Error())
			http.Error(w, "Unexpected error.", http.StatusInternalServerError)
		}

		i++
	}

	report := joinReport{JoinInfo: joins}

	response, err := json.Marshal(report)

	if err != nil {
		log.Errorf(ctx, "[Heartbeat] %v", err.Error())
		http.Error(w, "Unexpected error.", http.StatusInternalServerError)
		return
	}

	w.Write(response)

	log.Infof(ctx, "[Heartbeat] Server %v (%v, %v): %v/%v", server.UUID, server.Address, server.Port, server.PlayerCount, server.MaxPlayerCount)
}
