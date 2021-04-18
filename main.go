package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"rolflewis.com/spotify-status-sync/src/database"
	"rolflewis.com/spotify-status-sync/src/spotify"
	"rolflewis.com/spotify-status-sync/src/views"
)

var globalClient *http.Client

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
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

	router.POST("/slack/events", eventsEndpoint)
	router.POST("/slack/interactivity", interactivityEndpoint)

	// Create the global spotify client
	globalClient = http.DefaultClient

	// Database setup
	database.ConnectToDatabase()
	database.ValidateSchema()

	router.Run(":" + port)
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

func isSecureFromSlack(context *gin.Context) bool {
	version := "v0" // This is a slack constant currently
	timestampString := context.GetHeader("X-Slack-Request-Timestamp")

	if timestampString == "" {
		return false
	}

	timestamp, tsError := strconv.ParseInt(timestampString, 10, 64)
	if tsError != nil {
		return false
	}

	// Verify that this timestamp is in the last 2 minutes - mitigates replay attacks
	if math.Abs(time.Now().Sub(time.Unix(timestamp, 0)).Seconds()) > 2*60 {
		return false
	}

	// Copy the body buffer out, read it, and replace it
	var bodyBytes []byte
	if context.Request.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(context.Request.Body)
	}
	context.Request.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

	// Compute signature and compare
	totalString := version + ":" + strconv.FormatInt(timestamp, 10) + ":" + string(bodyBytes)
	hasher := hmac.New(sha256.New, []byte(os.Getenv("SLACK_SIGNING_KEY")))
	hasher.Write([]byte(totalString))
	mySignature := "v0=" + hex.EncodeToString(hasher.Sum(nil))
	providedSignature := context.GetHeader("X-Slack-Signature")

	// If the signature header was not provided, not sent by slack
	if providedSignature == "" {
		return false
	}

	// If the calculated and given sigs don't match, not sent by slack
	if !hmac.Equal([]byte(mySignature), []byte(providedSignature)) {
		return false
	}

	return true
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

func eventsEndpoint(context *gin.Context) {
	// Ensure is from slack and is secure
	if !isSecureFromSlack(context) {
		log.Println("Insecure request skipped.")
		return
	}

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

	if internalError(database.EnsureUserExists(event.User), context) {
		return
	}

	// If type is a app_home_opened, answer it
	if event.Type == "app_home_opened" {
		// Check if spotify has been connected yet for this session
		profileID, _, dbError := database.GetSpotifyForUser(event.User)
		if internalError(dbError, context) {
			return
		}

		log.Println("Spotify for user:", profileID)

		if profileID == "" { // Serve a new welcome screen
			pageError := views.CreateNewUserHomepage(event.User, globalClient)
			if internalError(pageError, context) {
				return
			}
		} else { // Serve an all-set screen
			pageError := views.CreateReturningHomepage(event.User, globalClient)
			if internalError(pageError, context) {
				return
			}
		}

		// Send an acknowledgment
		context.String(http.StatusOK, "Ok")
	} else {
		context.String(http.StatusBadRequest, "Not a supported event")
		log.Println("Not a supported event:", event)
	}
}

func disconnectHelper(user string) error {
	deleteError := database.DeleteAllDataForUser(user)
	if deleteError != nil {
		return deleteError
	}
	// After removing the data, reset the user's app home view back to the new user flow
	return views.CreateNewUserHomepage(user, globalClient)
}

// A generic struct that contains "header" data available on all interaction payloads
// Used to determine which more specific struct we can unmarshal the body into
type interactionHeader struct {
	Type string `json:"type"`
	User struct {
		ID string `json:"id"`
	} `json:"user"`
	Container struct {
		Type string `json:"type"`
	} `json:"container"`
}

type viewInteraction struct {
	Type string `json:"type"`
	Team struct {
		ID     string `json:"id"`
		Domain string `json:"domain"`
	} `json:"team"`
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		TeamID   string `json:"team_id"`
	} `json:"user"`
	APIAppID  string `json:"api_app_id"`
	Token     string `json:"token"`
	Container struct {
		Type   string `json:"type"`
		ViewID string `json:"view_id"`
	} `json:"container"`
	TriggerID string `json:"trigger_id"`
	View      struct {
		ID              string `json:"id"`
		TeamID          string `json:"team_id"`
		Type            string `json:"type"`
		PrivateMetadata string `json:"private_metadata"`
		CallbackID      string `json:"callback_id"`
		Hash            string `json:"hash"`
	}
	Actions []struct {
		ActionID string `json:"action_id"`
		BlockID  string `json:"block_id"`
		Text     struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Emoji bool   `json:"emoji"`
		} `json:"text"`
		Value     string `json:"value"`
		Type      string `json:"type"`
		Timestamp string `json:"action_ts"`
	}
}

func interactivityEndpoint(context *gin.Context) {
	// Ensure is from slack and is secure
	if !isSecureFromSlack(context) {
		log.Println("Insecure request skipped.")
		return
	}

	// Annoyingly, Slack sends the interaction data as json packed inside a URL form
	jsonBody, exists := context.GetPostForm("payload")
	if !exists {
		context.String(http.StatusBadRequest, "No payload provided.")
		return
	}

	// Parse the interaction header data
	var header interactionHeader
	headerParseError := json.Unmarshal([]byte(jsonBody), &header)
	if internalError(headerParseError, context) {
		log.Println("error while parsing header")
		return
	}

	// If this is not a view interaction, send an ack but ignore
	if header.Type != "block_actions" || header.Container.Type != "view" {
		context.String(http.StatusOK, "Ignored")
		return
	}

	// unmarshal to a more specific viewInteraction
	var interaction viewInteraction
	interactionParseError := json.Unmarshal([]byte(jsonBody), &interaction)
	if internalError(interactionParseError, context) {
		log.Println("error while parsing interaction")
		return
	}

	// If the interaction was not with the app home view, ack and ignore
	if interaction.View.Type != "home" {
		context.String(http.StatusOK, "Ignored")
		return
	}

	// Dispatch each button press to the correct helper function
	for _, action := range interaction.Actions {
		// Disconnect button
		if action.Type == "button" && action.ActionID == "spotify_disconnect_button" {
			disconnectError := disconnectHelper(interaction.User.ID)
			if internalError(disconnectError, context) {
				return
			}
		}
	}

	// Return an interaction success
	context.String(http.StatusOK, "Interaction Processed.")
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

	// if no state is somehow defined, bad request
	if user == "" {
		log.Println("No state/userid defined in callback request.")
		context.String(http.StatusBadRequest, "No state/userid defined in callback request.")
	}

	// Make sure we have a user record for the user
	if internalError(database.EnsureUserExists(user), context) {
		return
	}

	// Exchange code for tokens
	tokens, exchangeError := spotify.ExchangeCodeForTokens(code, globalClient)
	if internalError(exchangeError, context) {
		return
	}

	// Get the user's profile information
	profile, profileError := spotify.GetProfileForTokens(*tokens, globalClient)
	if internalError(profileError, context) {
		return
	}

	if profile == nil {
		log.Println("No profile returned from GET")
		context.String(http.StatusInternalServerError, "No profile returned from GET")
		return
	}

	// Save the information to the DB
	dbError := database.AddSpotifyToUser(user, *profile, *tokens)
	if internalError(dbError, context) {
		return
	}

	// update the homepage view
	viewError := views.CreateReturningHomepage(user, globalClient)
	if internalError(viewError, context) {
		return
	}

	context.String(http.StatusOK, "Signed in. You can close this window now.")
}
