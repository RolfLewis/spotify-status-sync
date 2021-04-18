package views

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var appURL = "https://spotify-status-sync.herokuapp.com/"
var slackAPIURL = "https://slack.com/api/"
var spotifyAuthURL = "https://accounts.spotify.com/"
var spotifyAPIURL = "https://api.spotify.com/v1/"

type viewPublishResponse struct {
	OK bool `json:"ok"`
}

func CreateNewUserHomepage(user string, client *http.Client) error {
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("client_id", os.Getenv("SPOTIFY_CLIENT_ID"))
	queryValues.Set("response_type", "code")
	queryValues.Set("redirect_uri", appURL+"spotify/callback")
	queryValues.Set("scope", "user-read-currently-playing")
	queryValues.Set("state", user)

	// Link to spotify OAuth page
	OAuthURL := spotifyAuthURL + "authorize?" + queryValues.Encode()

	// Update home view
	newView := `{
		"user_id": "` + user + `",
		"view":
		{
			"type": "home",
			"blocks": [
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "Hello! Thanks for using the Spotify / Slack Status Sync app. To get started, simply click the button below and log in through Spotify to connect your account."
					}
				},
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "*Log in with spotify here:*"
					},
					"accessory": {
						"type": "button",
						"text": {
							"type": "plain_text",
							"text": "Log in to Spotify",
							"emoji": false
						},
						"value": "spotify_login_button",
						"url": "` + OAuthURL + `",
						"action_id": "spotify_login_button"
					}
				}
			]
		}
	}`

	return updateHomepage(newView, client)
}

func CreateReturningHomepage(user string, client *http.Client) error {
	// Update home view
	newView := `{
		"user_id": "` + user + `",
		"view":
		{
			"type": "home",
			"blocks": [
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "You're all set."
					}
				},
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "*Disconnect your Spotify account here:*"
					},
					"accessory": {
						"type": "button",
						"text": {
							"type": "plain_text",
							"text": "Disconnect",
							"emoji": false
						},
						"value": "spotify_disconnect_button",
						"action_id": "spotify_disconnect_button"
					}
				}
			]
		}
	}`

	return updateHomepage(newView, client)
}

func updateHomepage(view string, client *http.Client) error {
	// Build request and send
	viewReq, viewReqError := http.NewRequest(http.MethodPost, slackAPIURL+"views.publish", strings.NewReader(view))
	if viewReqError != nil {
		return viewReqError
	}

	// Add the body headers
	viewReq.Header.Add("Content-Type", "application/json")
	viewReq.Header.Add("Content-Length", strconv.Itoa(len(view)))

	// Encode the authorization header
	viewReq.Header.Add("Authorization", "Bearer "+os.Getenv("SLACK_BEARER_TOKEN"))

	// Send the request
	viewResp, viewRespError := client.Do(viewReq)
	if viewRespError != nil {
		return viewRespError
	}
	defer viewResp.Body.Close()

	// Check status codes
	if viewResp.StatusCode != http.StatusOK {
		return errors.New("Non-200 status code from view.publish endpoint: " + strconv.Itoa(viewResp.StatusCode) + " / " + viewResp.Status)
	}

	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(viewResp.Body)
	if readError != nil {
		return readError
	}

	var responseObject viewPublishResponse
	jsonError := json.Unmarshal(jsonBytes, &responseObject)
	if jsonError != nil {
		return jsonError
	}

	if !responseObject.OK {
		return errors.New("Homepage update not reporting success.")
	}

	return nil
}
