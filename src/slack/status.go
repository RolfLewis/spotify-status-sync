package slack

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"

	"rolflewis.com/spotify-status-sync/src/database"
)

type UserProfile struct {
	IsOk    bool    `json:"ok"`
	Profile profile `json:"profile"`
	Error   string  `json:"error"`
}

type statusSetBody struct {
	Profile profile `json:"profile"`
}

type profile struct {
	StatusText       string `json:"status_text"`
	StatusEmoji      string `json:"status_emoji"`
	StatusExpiration int    `json:"status_expiration"`
}

func UpdateUserStatus(user string, newStatus string, client *http.Client) error {
	// Check if the last status we set is the same as this one
	lastStatus, dbReadError := database.GetStatusForUser(user)
	if dbReadError != nil {
		return dbReadError
	}
	// If this and last status match, return early
	if lastStatus == newStatus {
		return nil
	}
	// Read the status
	profile, readError := getUserStatus(user, client)
	if readError != nil {
		return readError
	}
	// Check if we can overwrite, and do so if we can
	if canOverwriteStatus(profile) {
		// Set the status in slack
		setError := setUserStatus(user, newStatus, client)
		if setError != nil {
			return setError
		}
		// Track the status change in the DB so we can avoid unneccesary checks
		dbWriteError := database.SetStatusForUser(user, newStatus)
		if dbWriteError != nil {
			return dbWriteError
		}
	}
	return nil
}

func canOverwriteStatus(profile *UserProfile) bool {
	// Don't overwrite if the status has an expiration
	if profile.Profile.StatusExpiration != 0 {
		return false
	}
	// Don't overwrite if the emoji is not :musical_note: or blank
	if profile.Profile.StatusEmoji != "" && profile.Profile.StatusEmoji != ":musical_note:" {
		return false
	}
	// Don't overwrite if the status isn't blank and doesn't contain a dash surrounded by spaces
	regex := regexp.MustCompile(`Listening to .* by .* on Spotify`)
	if profile.Profile.StatusText != "" && !regex.MatchString(profile.Profile.StatusText) {
		return false
	}

	return true
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
	// Get the token for this user
	token, tokenError := database.GetSlackForUser(user)
	if tokenError != nil {
		return nil, tokenError
	}
	// Add auth
	songReq.Header.Add("Authorization", "Bearer "+token)
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

func setUserStatus(user string, newStatus string, client *http.Client) error {
	// Create the json body
	bodyStruct := statusSetBody{
		Profile: profile{
			StatusText:       newStatus,
			StatusEmoji:      ":musical_note:",
			StatusExpiration: 0,
		},
	}
	// Marshal into string
	bodyBytes, jsonError := json.Marshal(bodyStruct)
	if jsonError != nil {
		return jsonError
	}

	// Get the auth and refresh tokens
	statusReq, statusReqError := http.NewRequest(http.MethodPost, os.Getenv("SLACK_API_URL")+"users.profile.set", ioutil.NopCloser(bytes.NewBuffer(bodyBytes)))
	if statusReqError != nil {
		return statusReqError
	}
	// Add the body headers
	statusReq.Header.Add("Content-Type", "application/json")
	statusReq.Header.Add("Content-Length", strconv.Itoa(len(bodyBytes)))
	// Get the token for this user
	token, tokenError := database.GetSlackForUser(user)
	if tokenError != nil {
		return tokenError
	}
	// Add auth
	statusReq.Header.Add("Authorization", "Bearer "+token)
	// Send the request
	statusResp, statusRespError := client.Do(statusReq)
	if statusRespError != nil {
		return statusRespError
	}
	defer statusResp.Body.Close()
	// Check status codes
	if statusResp.StatusCode != http.StatusOK {
		return errors.New("Non-200 status code from slack status set endpoint: " + strconv.Itoa(statusResp.StatusCode) + " / " + statusResp.Status)
	}
	// return success
	return nil
}
