package slack

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"

	"rolflewis.com/spotify-status-sync/src/database"
)

type UserProfile struct {
	IsOk    bool     `json:"ok"`
	Profile *profile `json:"profile,omitempty"`
	Error   *string  `json:"error"`
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
	if profile != nil && canOverwriteStatus(profile) {
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

func canOverwriteStatus(profile *profile) bool {
	// Don't overwrite if the status has an expiration
	if profile.StatusExpiration != 0 {
		return false
	}
	// Don't overwrite if the emoji is not :musical_note: or blank
	if profile.StatusEmoji != "" && profile.StatusEmoji != ":musical_note:" {
		return false
	}
	// Don't overwrite if the status isn't blank and isn't of our format
	// Has to be the most vague format we use, which is the catch-all over-limit fallback which only features name.
	regex := regexp.MustCompile(`Listening to .* on Spotify`)
	if profile.StatusText != "" && !regex.MatchString(profile.StatusText) {
		return false
	}

	return true
}

func getUserStatus(user string, client *http.Client) (*profile, error) {
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("user", user)
	// Get the token for this user
	token, tokenError := database.GetSlackForUser(user)
	if tokenError != nil {
		return nil, tokenError
	}
	authHeader := "Bearer " + token
	// Run request
	profile, requestError := profileRequestRunner(http.MethodGet, os.Getenv("SLACK_API_URL")+"users.profile.get?"+queryValues.Encode(), nil, authHeader, client)
	if requestError != nil {
		return nil, requestError
	}
	// if profile is nil, the token was revoked. Cleanup and exit.
	if profile == nil {
		log.Println("Cleaning up former user.")
		return nil, database.DeleteAllDataForUser(user)
	}
	return profile, nil
}

func setUserStatus(user string, newStatus string, client *http.Client) error {
	// Select the emoji for the status - if the status is blank, clear the emoji
	var emoji string
	if newStatus != "" {
		emoji = ":musical_note:"
	}
	// Create the json body
	bodyStruct := statusSetBody{
		Profile: profile{
			StatusText:       newStatus,
			StatusEmoji:      emoji,
			StatusExpiration: 0,
		},
	}
	// Marshal into string
	bodyBytes, jsonError := json.Marshal(bodyStruct)
	if jsonError != nil {
		return jsonError
	}
	// Get the token for this user
	token, tokenError := database.GetSlackForUser(user)
	if tokenError != nil {
		return tokenError
	}
	authHeader := "Bearer " + token
	// Run request
	profile, requestError := profileRequestRunner(http.MethodPost, os.Getenv("SLACK_API_URL")+"users.profile.set", bodyBytes, authHeader, client)
	if requestError != nil {
		return requestError
	}
	// if profile is nil, the token was revoked. Cleanup and exit.
	if profile == nil {
		log.Println("Cleaning up former user.")
		return database.DeleteAllDataForUser(user)
	}
	return nil
}

func profileRequestRunner(method string, url string, body []byte, auth string, client *http.Client) (*profile, error) {
	// Convert the body
	var bodyReader io.ReadCloser
	if body != nil {
		bodyReader = ioutil.NopCloser(bytes.NewBuffer(body))
	}
	// Get a new request
	statusReq, statusReqError := http.NewRequest(method, url, bodyReader)
	if statusReqError != nil {
		return nil, statusReqError
	}
	// Add the body headers
	if body != nil {
		statusReq.Header.Add("Content-Type", "application/json")
		statusReq.Header.Add("Content-Length", strconv.Itoa(len(body)))
	}
	// Add auth
	statusReq.Header.Add("Authorization", auth)
	// Send the request
	statusResp, statusRespError := client.Do(statusReq)
	if statusRespError != nil {
		return nil, statusRespError
	}
	defer statusResp.Body.Close()
	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(statusResp.Body)
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
		// If the error is token_revoked, trigger a cleanup for this user and exit gracefully
		if *profile.Error == "token_revoked" {
			return nil, nil
		} else {
			return nil, errors.New("Error reported from Slack profile endpoint: " + *profile.Error)
		}
	}
	return profile.Profile, nil
}
