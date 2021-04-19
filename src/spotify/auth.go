package spotify

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func ExchangeCodeForTokens(code string, isRefresh bool, client *http.Client) (map[string]interface{}, error) {
	// Set the query values
	queryValues := url.Values{}

	// Set the query urls differently depending on if this is a refresh or not
	if isRefresh {
		queryValues.Set("grant_type", "refresh_token")
		queryValues.Set("refresh_token", code)
	} else {
		queryValues.Set("grant_type", "authorization_code")
		queryValues.Set("code", code)
		queryValues.Set("redirect_uri", os.Getenv("APP_URL")+"spotify/callback")
	}
	urlEncodedBody := queryValues.Encode()

	// Get the auth and refresh tokens
	authReq, authReqError := http.NewRequest(http.MethodPost, os.Getenv("SPOTIFY_AUTH_URL")+"api/token", strings.NewReader(urlEncodedBody))
	if authReqError != nil {
		return nil, authReqError
	}

	// Add the body headers
	authReq.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	authReq.Header.Add("Content-Length", strconv.Itoa(len(urlEncodedBody)))

	// Encode the authorization header
	bytes := []byte(os.Getenv("SPOTIFY_CLIENT_ID") + ":" + os.Getenv("SPOTIFY_CLIENT_SECRET"))
	authReq.Header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString(bytes))

	// Send the request
	authResp, authRespError := client.Do(authReq)
	if authRespError != nil {
		return nil, authRespError
	}
	defer authResp.Body.Close()

	// Check status codes
	if authResp.StatusCode != http.StatusOK {
		return nil, errors.New("Non-200 status code from spotify auth endpoint: " + strconv.Itoa(authResp.StatusCode) + " / " + authResp.Status)
	}

	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(authResp.Body)
	if readError != nil {
		return nil, readError
	}

	var tokens map[string]interface{}
	jsonError := json.Unmarshal(jsonBytes, &tokens)
	if jsonError != nil {
		return nil, jsonError
	}

	return tokens, nil
}

func GetProfileForTokens(accessToken string, client *http.Client) (*string, error) {
	// Build request
	profReq, profReqError := http.NewRequest(http.MethodGet, os.Getenv("SPOTIFY_API_URL")+"me", nil)
	if profReqError != nil {
		return nil, profReqError
	}

	// Add auth
	profReq.Header.Add("Authorization", "Bearer "+accessToken)

	// Send the request
	profResp, profRespError := client.Do(profReq)
	if profRespError != nil {
		return nil, profRespError
	}
	defer profResp.Body.Close()

	// Check status codes
	if profResp.StatusCode != http.StatusOK {
		return nil, errors.New("Non-200 status code from profile endpoint: " + strconv.Itoa(profResp.StatusCode) + " / " + profResp.Status)
	}

	// Read the response
	jsonBytes, readError := ioutil.ReadAll(profResp.Body)
	if readError != nil {
		return nil, readError
	}

	// Cast to map
	var profile map[string]interface{}
	jsonError := json.Unmarshal(jsonBytes, &profile)
	if jsonError != nil {
		return nil, jsonError
	}

	// Get id from map
	id := profile["id"].(string)
	if id == "" {
		return nil, errors.New("ID from profile endpoint is empty")
	}

	return &id, nil
}
