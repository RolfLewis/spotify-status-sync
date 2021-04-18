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

type viewPublishResponse struct {
	OK bool `json:"ok"`
}

func CreateNewUserHomepage(user string, client *http.Client) error {
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("client_id", os.Getenv("SPOTIFY_CLIENT_ID"))
	queryValues.Set("response_type", "code")
	queryValues.Set("redirect_uri", os.Getenv("APP_URL")+"spotify/callback")
	queryValues.Set("scope", "user-read-currently-playing")
	queryValues.Set("state", user)

	// Link to spotify OAuth page
	OAuthURL := os.Getenv("SPOTIFY_AUTH_URL") + "authorize?" + queryValues.Encode()

	// Update home view
	newView := `{
		"user_id": "` + user + `",
		"view":
		{
			"type": "home",
			"blocks": [
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "*Description*"
					}
				},
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "This application serves a singular purpose. It syncs your currecntly playing spotify track into slack as your current status. It will not overwrite any other statuses like calendar status, manually set statuses, or OOO messages. It does not depend on Spotify Premium, so it will not cost you anything to use."
					}
				},
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "*Authorization / Security*"
					}
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "This application utilizes the industry standard OAuth2.0 flow to securely interact with both Spotify and Slack. When you select the login button below for Spotify, you log in to Spotify directly and authorize this application to interact with your account in very specific ways. These 'ways' are called scopes, and this application only asks for a scope which allows it to see your currently playing song. That's no access to private profile information, no song history, and no playlist access."
					}
				},
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "*Disconnecting*"
					}
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "You can disconnect your Slack and Spotify accounts at any time. Once you have them connected, this screen will update to include a 'disconnect' button. That button will immediately erase all information that the app saves about your accounts and clear your status if it is set by the app."
					}
				},
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "*Getting Started*"
					}
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "Click this button to be redirected to the spotify OAuth page:"
					},
					"accessory": {
						"type": "button",
						"text": {
							"type": "plain_text",
							"text": "Connect Spotify",
							"emoji": true
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
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "*Description*"
					}
				},
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "This application serves a singular purpose. It syncs your currecntly playing spotify track into slack as your current status. It will not overwrite any other statuses like calendar status, manually set statuses, or OOO messages. It does not depend on Spotify Premium, so it will not cost you anything to use."
					}
				},
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "*Authorization / Security*"
					}
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "This application utilizes the industry standard OAuth2.0 flow to securely interact with both Spotify and Slack. When you select the login button below for Spotify, you log in to Spotify directly and authorize this application to interact with your account in very specific ways. These 'ways' are called scopes, and this application only asks for a scope which allows it to see your currently playing song. That's no access to private profile information, no song history, and no playlist access."
					}
				},
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "*Disconnecting*"
					}
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "If you would like to disconnect your Slack and Spotify accounts, click the button below. That button will immediately erase all information that the app saves about your accounts and clear your status if it is set by the app."
					}
				},
				{
					"type": "divider"
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "*Erase Data and Disconnect*"
					}
				},
				{
					"type": "section",
					"text": {
						"type": "mrkdwn",
						"text": "Click this button to erase application data and disconnect:"
					},
					"accessory": {
						"type": "button",
						"text": {
							"type": "plain_text",
							"text": "Disconnect / Delete",
							"emoji": true
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
	viewReq, viewReqError := http.NewRequest(http.MethodPost, os.Getenv("SLACK_API_URL")+"views.publish", strings.NewReader(view))
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
