package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gofrs/uuid"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/taskqueue"
)

const (
	authenticateWithSteam       = true
	nonSteamAuthenticationToken = "SecretAuthToken"
	joinDelaySeconds            = 1
	mmUserResetMatchmakeTime    = 1
)

type mmPoll struct {
	Status int
}

type mmPollFull struct {
	Status        int
	JoinToken     string
	ServerAddress string
	ServerPort    int
}

// EnqueueHandler handles requests to queue for matchmaking
func enqueueHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	if r.Method != "GET" {
		log.Errorf(ctx, "[Enqueue] Invalid request method %v", r.Method)
		http.Error(w, "Invalid Request.", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()

	userID := q.Get("UserID")
	authToken := q.Get("AuthToken")
	region := q.Get("Region")

	if authenticateWithSteam {
		authenticated, steamID, err := steamAuth(ctx, authToken)

		if err != nil {
			log.Errorf(ctx, "[Enqueue] %v", err.Error())
			http.Error(w, "Unexpected error.", http.StatusInternalServerError)
			return
		} else if !authenticated {
			log.Errorf(ctx, "[Enqueue] Invalid Auth Token")
			http.Error(w, "Invalid Auth Token.", http.StatusUnauthorized)
			return
		} else if userID != steamID {
			log.Errorf(ctx, "[Enqueue] Invalid UserID")
			http.Error(w, "Invalid UserID.", http.StatusUnauthorized)
			return
		}
	} else if authToken != nonSteamAuthenticationToken {
		log.Errorf(ctx, "[Enqueue] Invalid Auth Token")
		http.Error(w, "Invalid Auth Token.", http.StatusUnauthorized)
		return
	}

	key, user, qErr := queryUser(ctx, "UserID =", userID)
	found := key != nil

	if qErr != nil && qErr != datastore.Done {
		log.Errorf(ctx, "[Enqueue] %v", qErr.Error())
		http.Error(w, "Unexpected error.", http.StatusInternalServerError)
		return
	}

	var mmtok string
	var err error

	if found {
		// Case where reconnecting
		timeSinceLastCheck := time.Now().Sub(user.CheckTime).Minutes()
		enoughTimeSinceLastCheck := timeSinceLastCheck >= mmUserResetMatchmakeTime
		canQueue := user.MMStatus == mmStatusInQueue || (user.MMStatus == mmStatusJoinedMatch && enoughTimeSinceLastCheck)

		if !canQueue {
			canQueue = (user.MMStatus == mmStatusMatchmakingFailed || user.MMStatus == mmStatusMatchmakingCancelled) && enoughTimeSinceLastCheck
		}

		log.Infof(ctx, "[Enqueue] Found user %v with token %v (Status=%v, CanQueue=%v, LastCheck=%v)", user.UserID, mmtok, user.MMStatus, canQueue, timeSinceLastCheck)

		if canQueue {
			mmtok = user.MMTok

			if enoughTimeSinceLastCheck {
				log.Infof(ctx, "[Enqueue] Requeuing user %v with token %v", user.UserID, mmtok)

				user.MMStatus = mmStatusInQueue
				user.CheckTime = time.Now()

				t := taskqueue.NewPOSTTask("/joinmatch", map[string][]string{"mmtok": {mmtok}, "region": {region}})
				t.Delay = time.Second * joinDelaySeconds
				_, err = taskqueue.Add(ctx, t, "default")

				if err != nil {
					log.Errorf(ctx, "[Enqueue] %v", err.Error())
					http.Error(w, "[Enqueue] Unexpected error.", http.StatusInternalServerError)
					return
				}
			} else {
				log.Infof(ctx, "[Enqueue] More time required to requeue user %v with token %v", user.UserID, mmtok)
			}
			_, err := datastore.Put(ctx, key, &user)
			if err != nil {
				log.Errorf(ctx, "[Enqueue] %v", err.Error())
				http.Error(w, "Unexpected error.", http.StatusInternalServerError)
				return
			}

		} else { // Case where matchmaking failed or was cancelled
			log.Infof(ctx, "[Enqueue] Invalid state for queuing: %v", user.MMStatus)
			http.Error(w, "Unexpected error.", http.StatusNotAcceptable)
			return
		}
	} else {
		mmtok = uuid.Must(uuid.NewV4()).String()

		user = mmUser{
			UserID:       userID,
			MMTok:        mmtok,
			MMStatus:     mmStatusInQueue,
			CreationTime: time.Now(),
			CheckTime:    time.Now(),
		}

		_, err := datastore.Put(ctx, datastore.NewIncompleteKey(ctx, "MMUser", nil), &user)
		if err != nil {
			log.Errorf(ctx, "[Enqueue] %v", err.Error())
			http.Error(w, "[Enqueue] Unexpected error.", http.StatusInternalServerError)
			return
		}

		t := taskqueue.NewPOSTTask("/joinmatch", map[string][]string{"mmtok": {mmtok}, "region": {region}})
		t.Delay = time.Second * joinDelaySeconds
		_, err = taskqueue.Add(ctx, t, "default")

		if err != nil {
			log.Errorf(ctx, "[Enqueue] %v", err.Error())
			http.Error(w, "[Enqueue] Unexpected error.", http.StatusInternalServerError)
			return
		}

		log.Infof(ctx, "[Enqueue] Added user %v with token %v in region %v", user.UserID, mmtok, region)
	}

	fmt.Fprintf(w, "%v", mmtok)
}

func dequeueHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	if r.Method != "GET" {
		log.Errorf(ctx, "[Dequeue] Invalid request method %v", r.Method)
		http.Error(w, "Invalid Request.", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()

	mmtok := q.Get("QueryToken")

	key, user, qErr := queryUser(ctx, "MMTok =", mmtok)
	found := key != nil

	if qErr != nil {
		log.Errorf(ctx, "[Dequeue] %v", qErr.Error())
		http.Error(w, "Unexpected error.", http.StatusInternalServerError)
		return
	}

	if !found {
		log.Errorf(ctx, "[Dequeue] Matchmaker Token Not Found.")
		http.Error(w, "Matchmaker Token Not Found.", http.StatusNotFound)
		return
	}

	user.MMStatus = mmStatusMatchmakingCancelled
	user.CheckTime = time.Now().Add(-(userRecordExpiryTime + 1) * time.Minute) // Guarantee stale for next cleanup

	_, err := datastore.Put(ctx, key, &user)
	if err != nil {
		log.Errorf(ctx, "[Enqueue] %v", err.Error())
		http.Error(w, "Unexpected error.", http.StatusInternalServerError)
		return
	}

	log.Infof(ctx, "[Dequeue] Marked user %v with token %v as cancelled", user.UserID, mmtok)

	fmt.Fprintf(w, "%v", mmtok)
}

func pollHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	if r.Method != "GET" {
		log.Errorf(ctx, "[Poll] Invalid request method %v", r.Method)
		http.Error(w, "Invalid Request.", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()

	mmtok := q.Get("QueryToken")

	key, user, qErr := queryUser(ctx, "MMTok =", mmtok)
	found := key != nil

	if qErr != nil {
		log.Errorf(ctx, "[Poll] %v", qErr.Error())
		http.Error(w, "Unexpected error.", http.StatusInternalServerError)
		return
	}

	if !found {
		log.Errorf(ctx, "[Poll] Matchmaker Token Not Found")
		http.Error(w, "Matchmaker Token Not Found.", http.StatusNotFound)
		return
	}

	if user.MMStatus == mmStatusInQueue { // Only update time if haven't found a match
		user.CheckTime = time.Now()

		_, err := datastore.Put(ctx, key, &user)
		if err != nil {
			log.Errorf(ctx, "[Poll] %v", err.Error())
			http.Error(w, "Unexpected error.", http.StatusInternalServerError)
			return
		}
	}

	status := user.MMStatus

	if status == mmStatusJoinedMatch {
		pollFull := mmPollFull{
			Status:        status,
			JoinToken:     user.JoinTok,
			ServerAddress: user.ServerAddr,
			ServerPort:    user.ServerPort,
		}

		response, err := json.Marshal(pollFull)

		if err != nil {
			log.Errorf(ctx, "[Poll] %v", err.Error())
			http.Error(w, "Unexpected error.", http.StatusInternalServerError)
			return
		}

		w.Write(response)

		log.Infof(ctx, "[Poll] User %v (%v): %v, JoinToken=%v", user.UserID, user.MMTok, user.MMStatus, user.JoinTok)
	} else {
		poll := mmPoll{
			Status: status,
		}

		response, err := json.Marshal(poll)

		if err != nil {
			log.Errorf(ctx, "[Poll] %v", err.Error())
			http.Error(w, "Unexpected error.", http.StatusInternalServerError)
			return
		}

		w.Write(response)

		log.Infof(ctx, "[Poll] User %v (%v): %v", user.UserID, user.MMTok, user.MMStatus)
	}
}
