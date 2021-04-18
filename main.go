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
var slackAPIURL = "https://slack.com/api/"
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

	router.GET("/spotify/callback", callbackFlow)
	router.POST("/slack/interactivity", interactivityEndpoint)
	router.POST("/slack/events", eventsEndpoint)

	// Create the global spotify client
	spotifyClient = http.DefaultClient

	// Database setup
	connectToDatabase()
	validateSchema()

	router.Run(":" + port)
}

type eventWrapper struct {
	Token     string `json:"token"`
	TeamID    string `json:"team_id"`
	APIAppID  string `json:"api_app_id"`
	Event     *event `json:"event"`
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
}

type event struct {
	Type      string `json:"type"`
	User      string `json:"user"`
	Channel   string `json:"channel"`
	Timestamp string `json:"event_ts"`
	Tab       string `json:"tab"`
}

type viewPublishResponse struct {
	OK bool `json:"ok"`
}

// Takes an error and handles logging it and reporting a 500. Returns true if error was non-nil
func internalError(err error, context *gin.Context) bool {
	if err != nil {
		log.Println(err.Error())
		context.String(http.StatusInternalServerError, err.Error())
		return true
	}
	return false
}

func eventsEndpoint(context *gin.Context) {
	// Parse the event
	var wrapper eventWrapper
	parseError := context.BindJSON(&wrapper)
	if internalError(parseError, context) {
		return
	}

	// If this is a challenge request, respond
	if wrapper.Type == "url_verification" {
		context.String(http.StatusOK, wrapper.Challenge)
		return
	}

	// Extract the inner event
	event := wrapper.Event

	// Make sure that a user record exists for the user
	exists, existsError := userExists(event.User)
	if internalError(existsError, context) {
		return
	}

	// Create a user record if needed
	if !exists {
		addNewUser(event.User)
	}

	// If type is a app_home_opened, answer it
	if event.Type == "app_home_opened" {
		// Check if spotify has been connected yet for this session
		profileID, _, dbError := getSpotifyForUser(event.User)
		if internalError(dbError, context) {
			return
		}

		log.Println("Spotify for user:", profileID)

		if profileID == "" { // Serve a new welcome screen
			createNewUserHomepage(event.User)
		} else { // Serve an all-set screen
			createReturningHomepage(event.User)
		}

		// Send an acknowledgment
		context.String(http.StatusOK, "Ok")
	} else {
		context.String(http.StatusBadRequest, "Not a supported event")
		log.Println("Not a supported event:", event)
	}
}

func interactivityEndpoint(context *gin.Context) {
	// var jsonData string
	// jsonError := context.BindJSON(&jsonData)
	// if jsonError != nil {
	// 	log.Println(jsonError.Error())
	// }
	// log.Println(jsonData)
	log.Println("interactive")
	context.String(http.StatusOK, "interactive")
}

func getLoginRedirectURL() string {
	return appURL + "spotify/callback"
}

func callbackFlow(context *gin.Context) {
	// Check for error from Spotify
	errorMsg := context.Query("error")
	if errorMsg != "" {
		log.Println(errorMsg)
		context.String(http.StatusInternalServerError, errorMsg)
		return
	}

	// Read auth code
	code := context.Query("code")

	// Read the user id (passed as state)
	user := context.Query("state")

	// Exchange code for tokens
	tokens, exchangeError := exchangeCodeForTokens(code)
	if internalError(exchangeError, context) {
		return
	}

	// Get the user's profile information
	profile, profileError := getProfileForTokens(*tokens)
	if internalError(profileError, context) {
		return
	}

	if profile == nil {
		log.Println("No profile returned from GET")
		context.String(http.StatusInternalServerError, "No profile returned from GET")
		return
	}

	// Save the information to the DB
	dbError := addSpotifyToUser(user, *profile, *tokens)
	if internalError(dbError, context) {
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

func createNewUserHomepage(user string) {
	// Set the query values
	queryValues := url.Values{}
	queryValues.Set("client_id", os.Getenv("SPOTIFY_CLIENT_ID"))
	queryValues.Set("response_type", "code")
	queryValues.Set("redirect_uri", getLoginRedirectURL())
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
							"emoji": true
						},
						"value": "login",
						"url": "` + OAuthURL + `",
						"action_id": "button-action"
					}
				}
			]
		}
	}`

	// Build request and send
	viewReq, viewReqError := http.NewRequest(http.MethodPost, slackAPIURL+"views.publish", strings.NewReader(newView))
	if viewReqError != nil {
		log.Println(viewReqError)
		return
	}

	// Add the body headers
	viewReq.Header.Add("Content-Type", "application/json")
	viewReq.Header.Add("Content-Length", strconv.Itoa(len(newView)))

	// Encode the authorization header
	viewReq.Header.Add("Authorization", "Bearer "+os.Getenv("SLACK_BEARER_TOKEN"))
	log.Println("Bearer " + os.Getenv("SLACK_BEARER_TOKEN"))

	// Send the request
	viewResp, viewRespError := spotifyClient.Do(viewReq)
	if viewRespError != nil {
		log.Println(viewRespError)
		return
	}
	defer viewResp.Body.Close()

	// Check status codes
	if viewResp.StatusCode != http.StatusOK {
		log.Println("Non-200 status code from view.publish endpoint: " + strconv.Itoa(viewResp.StatusCode) + " / " + viewResp.Status)
		return
	}

	// Read the tokens
	jsonBytes, readError := ioutil.ReadAll(viewResp.Body)
	if readError != nil {
		log.Println(readError)
		return
	}

	var responseObject viewPublishResponse
	jsonError := json.Unmarshal(jsonBytes, &responseObject)
	if jsonError != nil {
		log.Println(jsonError)
		return
	}

	if !responseObject.OK {
		log.Println("VIEW UPDATE NOT OKAY")
	}
}

func createReturningHomepage(user string) {
	log.Println(user, "in returing homepage")
}
