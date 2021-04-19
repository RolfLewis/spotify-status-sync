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
	}
}

func ExchangeCodeForToken(code string, client *http.Client) (*string, error) {
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("code", code)
	queryValues.Set("redirect_uri", os.Getenv("APP_URL")+"slack/callback")

	// Get the auth and refresh tokens
	authReq, authReqError := http.NewRequest(http.MethodPost, os.Getenv("SLACK_AUTH_URL")+"oauth.v2.access?"+queryValues.Encode(), nil)
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

	return &response.AuthedUser.AccessToken, nil
}
