package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
)

const (
	steamAPIURL     = "https://partner.steam-api.com/ISteamUserAuth/AuthenticateUserTicket/v1/"
	appIDPath       = "steam-appid.key"
	steamAPIKeyPath = "steam-api.key"
)

type steamAuthMessage struct {
	Response steamAuthResponse `json:"response"`
}

type steamAuthResponse struct {
	Params steamAuthParams `json:"params"`
	Error  steamAuthError  `json:"error"`
}

type steamAuthParams struct {
	Result          string `json:"result"`
	SteamID         string `json:"steamid"`
	OwnerSteamID    string `json:"ownersteamid"`
	VacBanned       bool   `json:"vacbanned"`
	PublisherBanned bool   `json:"publisherbanned"`
}

type steamAuthError struct {
	ErrorCode int    `json:"errorcode"`
	ErrorDesc string `json:"errordesc"`
}

var appID = getStringFromFile(appIDPath)
var steamAPIKey = getStringFromFile(steamAPIKeyPath)

func steamAuth(ctx context.Context, authToken string) (authenticated bool, steamID string, err error) {
	authenticated = false
	steamID = ""
	err = nil

	url, err := url.Parse(steamAPIURL)

	if err != nil {
		return
	}

	queryParams := url.Query()

	queryParams.Add("key", steamAPIKey)
	queryParams.Add("appid", appID)
	queryParams.Add("ticket", authToken)

	encodedQueryParams := queryParams.Encode()

	client := urlfetch.Client(ctx)

	fullURL := fmt.Sprintf("%v?%v", steamAPIURL, encodedQueryParams)
	req, err := http.NewRequest("GET", fullURL, nil)

	if err != nil {
		return
	}

	log.Infof(ctx, "[STEAM-AUTH] Sending Request...")

	resp, err := client.Do(req)

	if err != nil {
		log.Errorf(ctx, "[STEAM-AUTH] Auth Request Failed to Execute.")
		return
	}

	if resp.StatusCode != 200 {
		log.Errorf(ctx, "[STEAM-AUTH] Auth Request Failed: STATUS %v", resp.StatusCode)
		return
	}

	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return
	}

	log.Infof(ctx, "[STEAM-AUTH] Got: %v", string(responseBody))

	message := new(steamAuthMessage)
	err = json.Unmarshal(responseBody, &message)

	if err != nil {
		log.Errorf(ctx, "[STEAM-AUTH] Auth Request Failed to be Parsed.")
		return
	}

	if message.Response.Error.ErrorDesc != "" {
		log.Errorf(ctx, "[STEAM-AUTH] ERR: %v", message.Response.Error.ErrorDesc)
		authenticated = false
	} else {
		steamID = message.Response.Params.SteamID
		authenticated = true
	}

	return
}
