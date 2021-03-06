package spotify

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"rolflewis.com/spotify-status-sync/src/database"
)

type CurrentlyPlaying struct {
	IsPlaying            bool   `json:"is_playing"`
	CurrentlyPlayingType string `json:"currently_playing_type"`
	Item                 struct {
		Show struct {
			Name      string `json:"name"`
			Publisher string `json:"publisher"`
		} `json:"show"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
		IsExplicit bool   `json:"explicit"`
		ID         string `json:"id"`
		Name       string `json:"name"`
		Type       string `json:"type"`
	} `json:"item"`
}

// Returns the currently playing song struct, or error if error occurs. If the user is not playing anything or is in private session, currently playing is nil.
func GetCurrentlyPlayingForUser(user string, client *http.Client) (*CurrentlyPlaying, error) {
	// Get the data for this user
	_, tokens, tokensError := database.GetSpotifyForUser(user)
	if tokensError != nil {
		return nil, tokensError
	}
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("market", "from_token")
	queryValues.Set("additional_types", "episode")
	// Get the auth and refresh tokens
	songReq, songReqError := http.NewRequest(http.MethodGet, os.Getenv("SPOTIFY_API_URL")+"me/player/currently-playing?"+queryValues.Encode(), nil)
	if songReqError != nil {
		return nil, songReqError
	}
	// Add auth
	songReq.Header.Add("Authorization", "Bearer "+tokens[0])
	// Send the request
	songResp, songRespError := client.Do(songReq)
	if songRespError != nil {
		return nil, songRespError
	}
	defer songResp.Body.Close()
	// Check status codes
	if songResp.StatusCode != http.StatusOK && songResp.StatusCode != http.StatusNoContent {
		return nil, errors.New("Non-200/204 status code from auth endpoint: " + strconv.Itoa(songResp.StatusCode) + " / " + songResp.Status)
	}
	// If status code is 204, the user is not playing anything or is in a private session
	if songResp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(songResp.Body)
	if readError != nil {
		return nil, readError
	}
	// unmarshal into struct
	var current CurrentlyPlaying
	jsonError := json.Unmarshal(jsonBytes, &current)
	if jsonError != nil {
		return nil, jsonError
	}
	// Return success
	return &current, nil
}
