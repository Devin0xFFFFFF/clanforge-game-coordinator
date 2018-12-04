package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/signer/v4"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
)

const (
	authCredentialRegion      = "eu-west-1"
	authCredentialService     = "cf"
	allocateAPIPath           = "https://api.multiplay.co.uk/cfp/v1/server/allocate"
	deallocateAPIPath         = "https://api.multiplay.co.uk/cfp/v1/server/deallocate"
	allocationsAPIPath        = "https://api.multiplay.co.uk/cfp/v1/server/allocations"
	profileIDPath             = "clanforge-api-profile.key"
	clanforgeAPIAccessKeyPath = "clanforge-api-access.key"
	clanforgeAPISecretKeyPath = "clanforge-api-secret.key"
	naRegionName              = "na"
	naRegionIDPath            = "clanforge-api-region-na.key"
	euRegionName              = "eu"
	euRegionIDPath            = "clanforge-api-region-eu.key"
)

type allocateResponse struct {
	Success    bool                 `json:"success"`
	Messages   []string             `json:"messages"`
	Allocation allocateResponseInfo `json:"allocation"`
}

type allocateResponseInfo struct {
	ProfileID int    `json:"profileid"`
	UUID      string `json:"uuid"`
	Regions   string `json:"regions"`
	Created   string `json:"created"`
}

type deallocateResponse struct {
	UUID string `json:"uuid"`
}

type allocationsResponse struct {
	Success     bool                      `json:"success"`
	Messages    []string                  `json:"messages"`
	Allocations []allocationsResponseInfo `json:"allocations"`
}

type allocationsResponseInfo struct {
	ProfileID int    `json:"profileid"`
	UUID      string `json:"uuid"`
	Regions   string `json:"regions"`
	Created   string `json:"created"`
	Requested string `json:"requested"`
	Fulfilled string `json:"fulfilled"`
	ServerID  int    `json:"serverid"`
	FleetID   string `json:"fleetid"`
	RegionID  string `json:"regionid"`
	MachineID int    `json:"machineid"`
	IP        string `json:"ip"`
	GamePort  int    `json:"game_port"`
	Error     string `json:"error"`
}

var defaultProfileID = getStringFromFile(profileIDPath)
var clanforgeAPIAccessKey = getStringFromFile(clanforgeAPIAccessKeyPath)
var clanforgeAPISecretKey = getStringFromFile(clanforgeAPISecretKeyPath)
var naRegionID = getStringFromFile(naRegionIDPath)
var euRegionID = getStringFromFile(euRegionIDPath)

func getRegionID(region string) string {
	if region == naRegionName {
		return naRegionID
	} else if region == euRegionName {
		return euRegionID
	}

	return ""
}

func queryClanForgeAlloc(ctx context.Context, serverID, profileID, regionID string) (response allocateResponse, err error) {
	url, err := url.Parse(allocateAPIPath)

	if err != nil {
		return
	}

	queryParams := url.Query()

	queryParams.Add("profileid", profileID)
	queryParams.Add("regionid", regionID)
	queryParams.Add("uuid", serverID)

	encodedQueryParams := queryParams.Encode()

	responseCode, responseBody, err := queryClanForge(ctx, allocateAPIPath, encodedQueryParams)

	if err != nil {
		log.Errorf(ctx, "[Query-CF] ClanforgeRequest Failed, ERROR: %v", err)
		return
	}

	if responseCode != 200 {
		log.Errorf(ctx, "[Query-CF] ClanforgeRequest Failed, STATUS: %v", responseCode)
	}

	allocResponse := new(allocateResponse)
	err = json.Unmarshal(responseBody, &allocResponse)

	if err != nil {
		return
	}

	response = *allocResponse
	return
}

func queryClanForgeAllocations(ctx context.Context, serverID string) (response allocationsResponse, err error) {
	url, err := url.Parse(allocationsAPIPath)

	if err != nil {
		return
	}

	queryParams := url.Query()

	queryParams.Add("uuid", serverID)

	encodedQueryParams := queryParams.Encode()

	responseCode, responseBody, err := queryClanForge(ctx, allocationsAPIPath, encodedQueryParams)

	if err != nil {
		log.Errorf(ctx, "[Query-CF] ClanforgeRequest Failed, ERROR: %v", err)
		return
	}

	if responseCode != 200 {
		log.Errorf(ctx, "[Query-CF] ClanforgeRequest Failed, STATUS: %v", responseCode)
	}

	allocsResponse := new(allocationsResponse)
	err = json.Unmarshal(responseBody, &allocsResponse)

	if err != nil {
		return
	}

	response = *allocsResponse
	return
}

func queryClanForgeDealloc(ctx context.Context, serverID string) (response deallocateResponse, err error) {
	url, err := url.Parse(deallocateAPIPath)

	if err != nil {
		return
	}

	queryParams := url.Query()

	queryParams.Add("uuid", serverID)

	encodedQueryParams := queryParams.Encode()

	responseCode, responseBody, err := queryClanForge(ctx, deallocateAPIPath, encodedQueryParams)

	if err != nil {
		log.Errorf(ctx, "[Query-CF] ClanforgeRequest Failed, ERROR: %v", err)
		return
	}

	if responseCode != 200 {
		log.Errorf(ctx, "[Query-CF] ClanforgeRequest Failed, STATUS: %v", responseCode)
	}

	deallocResponse := new(deallocateResponse)
	err = json.Unmarshal(responseBody, &deallocResponse)

	if err != nil {
		return
	}

	response = *deallocResponse
	return
}

func queryClanForge(ctx context.Context, apiPath, queryParams string) (responseCode int, responseBody []byte, err error) {
	client := urlfetch.Client(ctx)

	fullURL := fmt.Sprintf("%v?%v", apiPath, queryParams)
	req, err := http.NewRequest("GET", fullURL, nil)

	if err != nil {
		return
	}

	credentials := credentials.NewStaticCredentials(clanforgeAPIAccessKey, clanforgeAPISecretKey, "")
	signer := v4.NewSigner(credentials)
	_, err = signer.Sign(req, nil, authCredentialService, authCredentialRegion, time.Now())

	log.Infof(ctx, "[Query-CF] Sending Request: %v", fullURL)

	resp, err := client.Do(req)

	if err != nil {
		return
	}

	responseCode = resp.StatusCode

	defer resp.Body.Close()

	responseBody, err = ioutil.ReadAll(resp.Body)

	if err != nil {
		return
	}

	return
}
