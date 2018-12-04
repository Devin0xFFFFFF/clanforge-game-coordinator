package main

import (
	"context"
	"net/http"
	"strconv"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
)

func freeAllocationsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	log.Infof(ctx, "[Free-Allocs] Running Free Allocations...")

	clearStuckAllocations(ctx, naRegionName)
	clearStuckAllocations(ctx, euRegionName)
}

func clearStuckAllocations(ctx context.Context, region string) {
	activeAllocsItem, err := memcache.Get(ctx, activeAllocationsKey+region)

	if err != nil && err != memcache.ErrCacheMiss {
		log.Errorf(ctx, "[Free-Allocs] %v", err.Error())
	}

	activeAllocs := 0

	if err == nil { // Found ActiveAllocations in cache
		parsedAllocs, err := strconv.Atoi(string(activeAllocsItem.Value))
		if err != nil {
			log.Errorf(ctx, "[Free-Allocs] %v", err.Error())
		}
		activeAllocs = parsedAllocs
	}

	if activeAllocs != 0 {
		memcache.Delete(ctx, activeAllocationsKey+region)
	}
}
