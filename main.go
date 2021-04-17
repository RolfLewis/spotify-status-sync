package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	_ "github.com/heroku/x/hmetrics/onload"
)

var appURL = "https://spotify-status-sync.herokuapp.com/"
var spotifyAuthURL = "https://accounts.spotify.com/"
var spotifyAPIURL = "https://api.spotify.com/v1/"
var spotifyClient *http.Client

type spotifyAuthResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type spotifyProfile struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
}

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	router := gin.New()
	router.Use(gin.Logger())
	router.LoadHTMLGlob("templates/*.tmpl.html")
	router.Static("/static", "static")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl.html", nil)
	})

	router.GET("/spotify/login", loginFlow)
	router.GET("/spotify/callback", callbackFlow)
	router.POST("/interactivity", interactivityEndpoint)
	router.POST("/slack/events", eventsEndpoint)

	// Create the global spotify client
	spotifyClient = http.DefaultClient

	// Database setup
	connectToDatabase()
	validateSchema()

	router.Run(":" + port)
}

type eventsChallenge struct {
	Token     string `json:"token"`
	Challenge string `json:"challenge"`
	Type      string `json:"type"`
}

type appHomeOpened struct {
	Type      string `json:"type"`
	User      string `json:"user"`
	Channel   string `json:"channel"`
	Timestamp string `json:"event_ts"`
	Tab       string `json:"tab"`
}

func eventsEndpoint(context *gin.Context) {
	// Attempt to parse as a challenge message first, just in case
	var jsonChallenge eventsChallenge
	challengeError := context.BindJSON(&jsonChallenge)
	if challengeError == nil {
		context.String(http.StatusOK, jsonChallenge.Challenge)
		return
	}

	// Event is not the challenge event, so try for an app_home_opened event first
	var openedHome appHomeOpened
	homeError := context.BindJSON(&openedHome)
	if homeError == nil {
		// Check if spotify has been connected yet for this session
		profileID, _, dbError := getSpotifyForUser(openedHome.User)
		if dbError != nil {
			context.String(http.StatusInternalServerError, dbError.Error())
			return
		}

		if profileID == "" { // Serve a new welcome screen
			context.JSON(http.StatusOK, `{
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
								"emoji": true
							},
							"value": "login",
							"url": "https://google.com",
							"action_id": "button-action"
						}
					}
				]
			}`)
		} else { // Serve an all-set screen
			context.JSON(http.StatusOK, `{
				"blocks": [
					{
						"type": "divider"
					},
					{
						"type": "section",
						"text": {
							"type": "mrkdwn",
							"text": "You're all set. Thanks."
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
								"emoji": true
							},
							"value": "login",
							"url": "https://google.com",
							"action_id": "button-action"
						}
					}
				]
			}`)
		}
	}
}

func interactivityEndpoint(context *gin.Context) {
	var jsonData string
	jsonError := context.BindJSON(&jsonData)
	if jsonError != nil {
		log.Println(jsonError.Error())
	}
	log.Println(jsonData)
	context.JSON(http.StatusOK, `{
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
						"emoji": true
					},
					"value": "login",
					"url": "https://google.com",
					"action_id": "button-action"
				}
			}
		]
	}`)
}

func getLoginRedirectURL() string {
	return appURL + "callback"
}

func loginFlow(context *gin.Context) {
	callbackURL := getLoginRedirectURL()

	// Read user id (passed as state)
	user := context.Query("state")

	// Check if spotify has been connected yet for this session
	profileID, tokens, dbError := getSpotifyForUser(user)
	if dbError != nil {
		context.String(http.StatusInternalServerError, dbError.Error())
		return
	}

	if profileID == "" { // Spotify has not been connected in this session yet
		// Set the query values
		queryValues := url.Values{}
		queryValues.Set("client_id", os.Getenv("SPOTIFY_CLIENT_ID"))
		queryValues.Set("response_type", "code")
		queryValues.Set("redirect_uri", url.PathEscape(callbackURL))
		queryValues.Set("scope", "user-read-currently-playing")

		// Redirect to spotify OAuth page
		OAuthURL := spotifyAuthURL + "/authorize?" + queryValues.Encode()
		context.Redirect(http.StatusPermanentRedirect, OAuthURL)
	} else { // Spotify already exists for this session
		context.String(http.StatusOK, "Spotify already connected for this session: "+profileID+" / "+tokens.AccessToken)
	}
}

func callbackFlow(context *gin.Context) {
	// Check for error from Spotify
	errorMsg := context.Query("error")
	if errorMsg != "" {
		context.String(http.StatusInternalServerError, errorMsg)
		return
	}

	// Read auth code
	code := context.Query("code")

	// Read the user id (passed as state)
	user := context.Query("state")

	// Exchange code for tokens
	tokens, exchangeError := exchangeCodeForTokens(code)
	if exchangeError != nil {
		context.String(http.StatusInternalServerError, exchangeError.Error())
		return
	}

	// Get the user's profile information
	profile, profileError := getProfileForTokens(*tokens)
	if profileError != nil {
		context.String(http.StatusInternalServerError, profileError.Error())
		return
	}

	if profile == nil {
		context.String(http.StatusInternalServerError, "No profile returned from GET")
		return
	}

	// Save the information to the DB
	dbError := addSpotifyToUser(user, *profile, *tokens)
	if dbError != nil {
		context.String(http.StatusInternalServerError, dbError.Error())
		return
	}

	context.String(http.StatusOK, user+" "+profile.DisplayName+" "+profile.ID)
}

func exchangeCodeForTokens(code string) (*spotifyAuthResponse, error) {
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("grant_type", "authorization_code")
	queryValues.Set("code", code)
	queryValues.Set("redirect_uri", getLoginRedirectURL())
	urlEncodedBody := queryValues.Encode()

	// Get the auth and refresh tokens
	authReq, authReqError := http.NewRequest(http.MethodPost, spotifyAuthURL+"api/token", strings.NewReader(urlEncodedBody))
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
	authResp, authRespError := spotifyClient.Do(authReq)
	if authRespError != nil {
		return nil, authRespError
	}
	defer authResp.Body.Close()

	// Check status codes
	if authResp.StatusCode != http.StatusOK {
		return nil, errors.New("Non-200 status code from auth endpoint: " + strconv.Itoa(authResp.StatusCode) + " / " + authResp.Status)
	}

	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(authResp.Body)
	if readError != nil {
		return nil, readError
	}

	var tokens spotifyAuthResponse
	jsonError := json.Unmarshal(jsonBytes, &tokens)
	if jsonError != nil {
		return nil, jsonError
	}

	return &tokens, nil
}

func getProfileForTokens(tokens spotifyAuthResponse) (*spotifyProfile, error) {
	// Build request
	profReq, profReqError := http.NewRequest(http.MethodGet, spotifyAPIURL+"me", nil)
	if profReqError != nil {
		return nil, profReqError
	}

	// Add auth
	profReq.Header.Add("Authorization", "Bearer "+tokens.AccessToken)

	// Send the request
	profResp, profRespError := spotifyClient.Do(profReq)
	if profRespError != nil {
		return nil, profRespError
	}
	defer profResp.Body.Close()

	// Check status codes
	if profResp.StatusCode != http.StatusOK {
		return nil, errors.New("Non-200 status code from profile endpoint: " + strconv.Itoa(profResp.StatusCode) + " / " + profResp.Status)
	}

	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(profResp.Body)
	if readError != nil {
		return nil, readError
	}

	var profile spotifyProfile
	jsonError := json.Unmarshal(jsonBytes, &profile)
	if jsonError != nil {
		return nil, jsonError
	}

	return &profile, nil
}

func createAndSendHomepageForUser(user string) {

}
