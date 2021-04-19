package spotify

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"rolflewis.com/spotify-status-sync/src/database"
)

type CurrentlyPlaying struct {
	Timestamp            int    `json:"timestamp"`
	ProgressMilliseconds int    `json:"progress_ms"`
	IsPlaying            bool   `json:"is_playing"`
	CurrentlyPlayingType string `json:"currently_playing_type"`
	Item                 struct {
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"artists"`
		DurationMilliseconds int    `json:"duration_ms"`
		IsExplicit           bool   `json:"explicit"`
		ID                   string `json:"id"`
		Name                 string `json:"name"`
		Popularity           int    `json:"popularity"`
		Type                 string `json:"type"`
	} `json:"item"`
}

func GetAllCurrentlyPlayings(client *http.Client) (int, error) {
	// Get all users who have spotify connected
	users, usersError := database.GetAllConnectedUsers()
	if usersError != nil {
		return 0, usersError
	}
	// Get currently playing for each user
	for index, user := range users {
		current, currentError := getCurrentlyPlayingSongForUser(user, client)
		if currentError != nil {
			return index, currentError
		}
		log.Println(current.Item.Name, current.Item.Artists[0].Name)
	}
	// return success
	return len(users), nil
}

// Returns the currently playing song struct, or error if error occurs. If the user is not playing anything or is in private session, currently playing is nil.
func getCurrentlyPlayingSongForUser(user string, client *http.Client) (*CurrentlyPlaying, error) {
	// Get the data for this user
	_, tokens, tokensError := database.GetSpotifyForUser(user)
	if tokensError != nil {
		return nil, tokensError
	}
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("market", "from_token")
	// Get the auth and refresh tokens
	songReq, songReqError := http.NewRequest(http.MethodGet, os.Getenv("SPOTIFY_AUTH_URL")+"api/token?"+queryValues.Encode(), nil)
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
