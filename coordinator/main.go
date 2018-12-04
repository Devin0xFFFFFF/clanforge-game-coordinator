package main

import (
	"net/http"

	"google.golang.org/appengine"
)

func main() {
	http.HandleFunc("/enqueue", enqueueHandler)
	http.HandleFunc("/dequeue", dequeueHandler)
	http.HandleFunc("/poll", pollHandler)
	http.HandleFunc("/joinmatch", joinMatchHandler)
	http.HandleFunc("/heartbeat", heartbeatHandler)
	http.HandleFunc("/manage", manageServersHandler)
	http.HandleFunc("/alloc", allocateServerHandler)
	http.HandleFunc("/allocation", allocationsServerHandler)
	http.HandleFunc("/dealloc", deallocateServerHandler)
	http.HandleFunc("/freeallocs", freeAllocationsHandler)
	http.HandleFunc("/stats", statsHandler)
	appengine.Main()
}
