package slack

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"rolflewis.com/spotify-status-sync/src/database"
)

type viewPublishResponse struct {
	OK bool `json:"ok"`
}

func UpdateHome(user string, client *http.Client) error {
	// Check if spotify has been connected yet for this user
	profileID, _, dbError := database.GetSpotifyForUser(user)
	if dbError != nil {
		return dbError
	}

	// Check if the user has authorized slack
	token, getError := database.GetSlackForUser(user)
	if getError != nil {
		return getError
	}

	// Control vars
	spotifyConnected := (profileID != "")
	slackConnected := (token != "")
	noneConnected := !(spotifyConnected || slackConnected)
	bothConnected := (spotifyConnected && slackConnected)

	// Start the view up to the point that something becomes dynamic
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
						"text": "This application serves a singular purpose. It syncs your currently playing spotify track into slack as your current status. It will not overwrite any other statuses like calendar status, manually set statuses, or OOO messages. It does not depend on Spotify Premium, so it will not cost you anything to use."
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
				},`

	if !noneConnected {
		newView += `{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "If you would like to disconnect your Slack and Spotify accounts, click the button below. That button will immediately erase all information that the app saves about your accounts and clear your status if it is set by the app."
			}
		},`
	} else {
		newView += `{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "You can disconnect your Slack and Spotify accounts at any time. Once you have them connected, this page will update to include a 'disconnect' button. That button will immediately erase all information that the app saves about your accounts and clear your status if it is set by the app."
			}
		},`
	}

	newView += `{
		"type": "divider"
	},`

	if !bothConnected {
		newView += `{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*Getting Started*"
			}
		},`

		if slackConnected {
			newView += `{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": "Slack is connected and ready to go!"
				}
			},`
		} else {
			// Set the query values
			slackQueryValues := url.Values{}
			slackQueryValues.Set("client_id", os.Getenv("SLACK_CLIENT_ID"))
			slackQueryValues.Set("redirect_uri", os.Getenv("APP_URL")+"slack/callback")
			slackQueryValues.Set("user_scope", "users.profile:read,users.profile:write")
			slackQueryValues.Set("state", user)

			// Link to spotify OAuth page
			slackOAuthURL := os.Getenv("SLACK_AUTH_URL") + "authorize?" + slackQueryValues.Encode()

			newView += `{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": "Click this button to be redirected to the Slack OAuth page:"
				},
				"accessory": {
					"type": "button",
					"text": {
						"type": "plain_text",
						"text": "Authorize Slack",
						"emoji": true
					},
					"value": "slack_login_button",
					"url": "` + slackOAuthURL + `",
					"action_id": "slack_login_button"
				}
			},`
		}

		if spotifyConnected {
			newView += `{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": "Spotify is connected and ready to go!"
				}
			}`
		} else {
			// Set the query values
			spotifyQueryValues := url.Values{}
			spotifyQueryValues.Set("client_id", os.Getenv("SPOTIFY_CLIENT_ID"))
			spotifyQueryValues.Set("response_type", "code")
			spotifyQueryValues.Set("redirect_uri", os.Getenv("APP_URL")+"spotify/callback")
			spotifyQueryValues.Set("scope", "user-read-currently-playing")
			spotifyQueryValues.Set("state", user)

			// Link to spotify OAuth page
			spotifyOAuthURL := os.Getenv("SPOTIFY_AUTH_URL") + "authorize?" + spotifyQueryValues.Encode()

			newView += `{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": "Click this button to be redirected to the Spotify OAuth page:"
				},
				"accessory": {
					"type": "button",
					"text": {
						"type": "plain_text",
						"text": "Connect Spotify",
						"emoji": true
					},
					"value": "spotify_login_button",
					"url": "` + spotifyOAuthURL + `",
					"action_id": "spotify_login_button"
				}
			}`
		}

		if !noneConnected {
			newView += `,{
				"type": "divider"
			},`
		}
	}

	if !noneConnected {
		newView += `{
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
		}`
	}

	newView += "]}}" // Close blocks array, view object, and then json
	log.Println(newView)
	return updateHomeHelper(newView, client)
}

func updateHomeHelper(view string, client *http.Client) error {
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