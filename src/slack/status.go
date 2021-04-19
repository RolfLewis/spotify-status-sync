package slack

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

type UserProfile struct {
	IsOk    bool `json:"ok"`
	Profile struct {
		StatusText       string `json:"status_text"`
		StatusEmoji      string `json:"status_emoji"`
		StatusExpiration string `json:"status_expiration"`
	} `json:"profile"`
	Error string `json:"error"`
}

func UpdateUserStatus(user string, newStatus string, client *http.Client) error {
	// WIP
	return nil
}

func getUserStatus(user string, client *http.Client) (*UserProfile, error) {
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("user", user)
	// Get the auth and refresh tokens
	songReq, songReqError := http.NewRequest(http.MethodGet, os.Getenv("SLACK_API_URL")+"users.profile.get?"+queryValues.Encode(), nil)
	if songReqError != nil {
		return nil, songReqError
	}
	// Add auth
	songReq.Header.Add("Authorization", "Bearer "+os.Getenv("SLACK_BEARER_TOKEN"))
	// Send the request
	songResp, songRespError := client.Do(songReq)
	if songRespError != nil {
		return nil, songRespError
	}
	defer songResp.Body.Close()
	// Check status codes
	if songResp.StatusCode != http.StatusOK {
		return nil, errors.New("Non-200 status code from slack profile endpoint: " + strconv.Itoa(songResp.StatusCode) + " / " + songResp.Status)
	}
	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(songResp.Body)
	if readError != nil {
		return nil, readError
	}
	// unmarshal into struct
	var profile UserProfile
	jsonError := json.Unmarshal(jsonBytes, &profile)
	if jsonError != nil {
		return nil, jsonError
	}
	// Check the ok field
	if !profile.IsOk {
		return nil, errors.New("Error reported from Slack profile endpoint: " + profile.Error)
	}
	// Return success
	return &profile, nil
}
