package slack

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

type slackAuthResponse struct {
	IsOk        bool   `json:"ok"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	BotUserID   string `json:"bot_user_id"`
	AppID       string `json:"app_id"`
	Team        struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"team"`
	Enterprise struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"enterprise"`
	AuthedUser struct {
		ID          string `json:"id"`
		Scope       string `json:"scope"`
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	} `json:"authed_user"`
}

func ExchangeCodeForToken(code string, client *http.Client) (*slackAuthResponse, error) {
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("code", code)
	queryValues.Set("redirect_uri", os.Getenv("APP_URL")+"slack/callback")

	// Get the auth and refresh tokens
	authReq, authReqError := http.NewRequest(http.MethodPost, os.Getenv("SLACK_API_URL")+"oauth.v2.access?"+queryValues.Encode(), nil)
	if authReqError != nil {
		return nil, authReqError
	}

	// Encode the authorization header
	bytes := []byte(os.Getenv("SLACK_CLIENT_ID") + ":" + os.Getenv("SLACK_CLIENT_SECRET"))
	authReq.Header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString(bytes))

	// Send the request
	authResp, authRespError := client.Do(authReq)
	if authRespError != nil {
		return nil, authRespError
	}
	defer authResp.Body.Close()

	// Check status codes
	if authResp.StatusCode != http.StatusOK {
		return nil, errors.New("Non-200 status code from slack auth endpoint: " + strconv.Itoa(authResp.StatusCode) + " / " + authResp.Status)
	}

	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(authResp.Body)
	if readError != nil {
		return nil, readError
	}

	var response slackAuthResponse
	jsonError := json.Unmarshal(jsonBytes, &response)
	if jsonError != nil {
		return nil, jsonError
	}

	return &response, nil
}

// func RevokeUserToken(user string, client *http.Client) error {
// 	// internal structs
// 	type responseStruct struct {
// 		OK      bool   `json:"ok"`
// 		Revoked bool   `json:"revoked"`
// 		Error   string `json:"error"`
// 	}

// 	// Get the token for the user
// 	token, tokenError := database.GetSlackForUser(user)
// 	if tokenError != nil {
// 		return tokenError
// 	}
// 	// Create a request
// 	authReq, authReqError := http.NewRequest(http.MethodGet, os.Getenv("SLACK_API_URL")+"auth.revoke", nil)
// 	if authReqError != nil {
// 		return authReqError
// 	}

// 	// Encode the authorization header
// 	authReq.Header.Add("Authorization", "Bearer "+token)

// 	// Send the request
// 	authResp, authRespError := client.Do(authReq)
// 	if authRespError != nil {
// 		return authRespError
// 	}
// 	defer authResp.Body.Close()

// 	// Check status codes
// 	if authResp.StatusCode != http.StatusOK {
// 		return errors.New("Non-200 status code from slack auth revoke endpoint: " + strconv.Itoa(authResp.StatusCode) + " / " + authResp.Status)
// 	}

// 	// Read the tokens
// 	jsonBytes, readError := ioutil.ReadAll(authResp.Body)
// 	if readError != nil {
// 		return readError
// 	}

// 	// Cast to struct
// 	var response responseStruct
// 	jsonError := json.Unmarshal(jsonBytes, &response)
// 	if jsonError != nil {
// 		return jsonError
// 	}

// 	// Check errors
// 	if !response.OK {
// 		return errors.New(response.Error)
// 	}

// 	// Make sure was actually revoked
// 	if !response.Revoked {
// 		return errors.New("Auth was not revoked.")
// 	}
// 	return nil
// }
