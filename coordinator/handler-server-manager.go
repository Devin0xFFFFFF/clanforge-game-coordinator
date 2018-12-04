package main

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
	"google.golang.org/appengine/taskqueue"
)

const (
	serverFullThreshold             = 0.8
	allocateNewServerThreshold      = 0.75
	serverTimeoutExpirationDuration = 60
	serverAgeExpirationDuration     = 60
	activeAllocationsKey            = "ServerManager-ActiveAllocations"
	defaultMaxPlayers               = 64
	serverInitDelaySeconds          = 10
	maxAllocateAttempts             = 4
	maxAllocationCheckAttempts      = 4
	maxServersPerRegion             = 10
)

type gameServerReport struct {
	UUID    string
	State   int
	Full    bool
	Expired bool
}

type serverStats struct {
	Region              string
	Timestamp           time.Time
	TotalServers        int
	TotalCurrentPlayers int
	TotalMaxPlayers     int
}

func manageServersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	c := make(chan int)

	go manageRegionServers(ctx, naRegionName, c)
	go manageRegionServers(ctx, euRegionName, c)

	<-c
	<-c
}

func manageRegionServers(ctx context.Context, region string, completeChan chan int) {
	log.Infof(ctx, "[Manage] Managing region %v servers", region)

	keys := make(map[string]*datastore.Key)

	var server gameServer

	q := datastore.NewQuery("GameServer").Filter("Region =", region)

	serverCount, err := q.Count(ctx)

	if err != nil {
		log.Errorf(ctx, "[Manage] %v", err.Error())
		completeChan <- 0
		return
	}

	stats := serverStats{Region: region, Timestamp: time.Now()}
	reports := make([]gameServerReport, serverCount)
	i := 0

	for t := q.Run(ctx); ; {
		key, err := t.Next(&server)
		if err == datastore.Done {
			break
		}
		if err != nil {
			log.Errorf(ctx, "[Manage] %v", err.Error())
			completeChan <- 0
			return
		}

		stats.TotalServers++
		stats.TotalCurrentPlayers += server.PlayerCount
		stats.TotalMaxPlayers += server.MaxPlayerCount

		keys[server.UUID] = key // Store mapping of UUID to datastore key for later update

		timeDelta := time.Now().Sub(server.CheckTime)
		timedOut := timeDelta.Seconds() > serverTimeoutExpirationDuration

		timeDelta = time.Now().Sub(server.CheckTime)
		tooOld := timeDelta.Minutes() >= serverAgeExpirationDuration

		expired := server.State == serverStateTerminating || timedOut || tooOld

		reports[i] = gameServerReport{
			UUID:    server.UUID,
			State:   server.State,
			Full:    server.Fill >= serverFullThreshold,
			Expired: expired,
		}

		i++
	}

	_, err = datastore.Put(ctx, datastore.NewIncompleteKey(ctx, "ServerStats", nil), &stats)
	if err != nil {
		log.Errorf(ctx, "[Manage] %v", err.Error())
		return
	}

	// Determine expirations and fill counts

	fullServerCount := 0
	activeServerCount := 0

	var expiredServerKeys []*datastore.Key

	for _, report := range reports {
		if report.Expired {
			log.Infof(ctx, "[Manage] Scheduling expiration of server %v", report.UUID)
			expiredServerKeys = append(expiredServerKeys, keys[report.UUID])

			t := taskqueue.NewPOSTTask("/dealloc", map[string][]string{"serverID": {report.UUID}})
			_, err := taskqueue.Add(ctx, t, "coordinator-deallocate")
			if err != nil {
				log.Errorf(ctx, "[Manage] %v", err.Error())
				return
			}
		} else if report.State == serverStateInitializing || report.State == serverStateActive {
			if report.Full {
				fullServerCount++
				activeServerCount++
			} else {
				activeServerCount++
			}
		}
	}

	// Remove all GameServer records that are expired

	err = datastore.DeleteMulti(ctx, expiredServerKeys)

	if err != nil {
		log.Errorf(ctx, "[Manage] %v", err.Error())
		completeChan <- 0
		return
	}

	// Perform allocation requests

	activeAllocsItem, err := memcache.Get(ctx, activeAllocationsKey+region)

	if err != nil && err != memcache.ErrCacheMiss {
		log.Errorf(ctx, "[Manage] %v", err.Error())
		completeChan <- 0
		return
	}

	activeAllocs := 0

	if err == nil { // Found ActiveAllocations in cache
		parsedAllocs, err := strconv.Atoi(string(activeAllocsItem.Value))
		if err != nil {
			log.Errorf(ctx, "[Manage] %v", err.Error())
			completeChan <- 0
			return
		}
		activeAllocs = parsedAllocs
	}

	fullServersRatio := float64(fullServerCount) / float64(activeServerCount+activeAllocs)

	if math.IsNaN(fullServersRatio) {
		fullServersRatio = 0
	}

	log.Infof(ctx, "[Manage] Region %v Server Fill Stats (Full/Partial/Allocating/Total - Fill Ratio): %v/%v/%v/%v - %.f",
		region, fullServerCount, activeServerCount-fullServerCount, activeAllocs, activeServerCount+activeAllocs, fullServersRatio)

	if activeServerCount == 0 || fullServersRatio > allocateNewServerThreshold {
		if activeServerCount >= maxServersPerRegion {
			log.Infof(ctx, "[Manage] Max Servers In %v Reached, stopping allocation.", region)
			return
		}

		log.Infof(ctx, "[Manage] Scheduling new server for allocation")

		t := taskqueue.NewPOSTTask("/alloc", map[string][]string{"region": {region}})
		_, err := taskqueue.Add(ctx, t, "coordinator-allocate")
		if err != nil {
			log.Errorf(ctx, "[Manage] %v", err.Error())
			completeChan <- 0
			return
		}

		_, err = memcache.Increment(ctx, activeAllocationsKey+region, 1, 0)
		if err != nil {
			log.Errorf(ctx, "[Manage] %v", err.Error())
			completeChan <- 0
			return
		}
	}

	completeChan <- 1
}

func allocateServerHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	var err error

	region := r.FormValue("region")
	regionID := getRegionID(region)

	attemptsHeader := r.Header.Get("X-AppEngine-TaskRetryCount")
	attempts, err := strconv.Atoi(attemptsHeader)

	if err != nil {
		log.Errorf(ctx, "[Alloc] %v", err.Error())
		http.Error(w, "Internal error.", http.StatusInternalServerError)
		return
	}

	if attempts > maxAllocateAttempts {
		log.Infof(ctx, "[Alloc] Allocate max attempts reached for region %v...", region)

		_, err = memcache.Increment(ctx, activeAllocationsKey+region, -1, 1)
		if err != nil {
			log.Errorf(ctx, "[Alloc] %v", err.Error())
			http.Error(w, "Internal error.", http.StatusInternalServerError)
			return
		}
	}

	serverID := uuid.Must(uuid.NewV4()).String()

	log.Infof(ctx, "[Alloc] Allocating server %v in region %v...", serverID, region)

	var allocResponse allocateResponse

	if appengine.IsDevAppServer() { // Don't call ClanForge in testing
		allocResponse = allocateResponse{Success: true}
		err = nil
	} else { // Request server from Multiplay (ClanForge)
		allocResponse, err = queryClanForgeAlloc(ctx, serverID, profileID, regionID)
	}

	if err != nil {
		log.Errorf(ctx, "[Alloc] %v", err.Error())
		return
	}

	if !allocResponse.Success {
		log.Errorf(ctx, "[Alloc] Allocation Failed: %v", strings.Join(allocResponse.Messages[:], ","))
		return
	}

	t := taskqueue.NewPOSTTask("/allocation", map[string][]string{"serverID": {serverID}, "region": {region}})
	t.Delay = time.Second * serverInitDelaySeconds
	_, err = taskqueue.Add(ctx, t, "coordinator-allocations")
	if err != nil {
		log.Errorf(ctx, "[Alloc] %v", err.Error())
		return
	}

	log.Infof(ctx, "[Alloc] Allocated new server %v in region  %v", serverID, region)
}

func allocationsServerHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	serverID := r.FormValue("serverID")
	region := r.FormValue("region")

	log.Infof(ctx, "[Allocation] Checking server allocation %v...", serverID)

	var allocationResponse allocationsResponse
	var err error

	if appengine.IsDevAppServer() { // Don't call ClanForge in testing
		allocationResponse = allocationsResponse{
			Allocations: []allocationsResponseInfo{
				allocationsResponseInfo{
					IP:       "127.0.0.1",
					GamePort: 7777,
				},
			},
			Success: true,
		}
		err = nil
	} else {
		allocationResponse, err = queryClanForgeAllocations(ctx, serverID)
	}

	if err != nil {
		log.Errorf(ctx, "[Allocation] %v", err.Error())
		return
	}

	if !allocationResponse.Success {
		log.Errorf(ctx, "[Allocation] Allocation Failed: %v", strings.Join(allocationResponse.Messages[:], ","))
		return
	}

	info := allocationResponse.Allocations[0]

	if info.IP == "" || info.GamePort == 0 {
		log.Errorf(ctx, "[Allocation] Allocation not ready: %v", serverID)

		attemptsHeader := r.Header.Get("X-AppEngine-TaskRetryCount")
		attempts, err := strconv.Atoi(attemptsHeader)

		if err != nil {
			log.Errorf(ctx, "[Allocation] %v", err.Error())
			http.Error(w, "Internal error.", http.StatusInternalServerError)
			return
		}

		if attempts >= maxAllocationCheckAttempts {
			log.Errorf(ctx, "[Allocation] Allocation check max attempts reached, deallocating server: %v", serverID)

			t := taskqueue.NewPOSTTask("/dealloc", map[string][]string{"serverID": {serverID}})
			_, err := taskqueue.Add(ctx, t, "coordinator-deallocate")
			if err != nil {
				log.Errorf(ctx, "[Manage] %v", err.Error())
				http.Error(w, "Internal error.", http.StatusInternalServerError)
				return
			}

			_, err = memcache.Increment(ctx, activeAllocationsKey+region, -1, 1)
			if err != nil {
				log.Errorf(ctx, "[Allocation] %v", err.Error())
				http.Error(w, "Internal error.", http.StatusInternalServerError)
				return
			}
		}

		http.Error(w, "Internal error.", http.StatusInternalServerError)
		return
	}

	server := gameServer{
		UUID:           serverID,
		Address:        info.IP,
		Port:           info.GamePort,
		Region:         region,
		State:          serverStateInitializing,
		CreationTime:   time.Now(),
		CheckTime:      time.Now(),
		PlayerCount:    0,
		MaxPlayerCount: defaultMaxPlayers,
		Fill:           0,
	}

	_, err = datastore.Put(ctx, datastore.NewIncompleteKey(ctx, "GameServer", nil), &server)
	if err != nil {
		log.Errorf(ctx, "[Allocation] %v", err.Error())
		return
	}

	_, err = memcache.Increment(ctx, activeAllocationsKey+region, -1, 1)
	if err != nil {
		log.Errorf(ctx, "[Allocation] %v", err.Error())
		http.Error(w, "Internal error.", http.StatusInternalServerError)
		return
	}

	log.Infof(ctx, "[Allocation] Confirmed new server %v (%v, %v) in region  %v", server.UUID, server.Address, server.Port, region)
}

func deallocateServerHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	serverID := r.FormValue("serverID")

	log.Infof(ctx, "[Dealloc] Deallocating server %v...", serverID)

	var deallocResponse deallocateResponse
	var err error

	if appengine.IsDevAppServer() { // Don't call ClanForge in testing
		deallocResponse = deallocateResponse{UUID: serverID}
	} else {
		deallocResponse, err = queryClanForgeDealloc(ctx, serverID)
	}

	if err != nil {
		log.Errorf(ctx, "[Dealloc] %v", err.Error())
	} else if deallocResponse.UUID == "" {
		log.Infof(ctx, "[Dealloc] Deallocated server %v", serverID)
	}
}
