package routes

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"rolflewis.com/spotify-status-sync/src/database"
	"rolflewis.com/spotify-status-sync/src/slack"
	"rolflewis.com/spotify-status-sync/src/util"
)

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

func EventsEndpoint(context *gin.Context, client *http.Client) {
	// Ensure is from slack and is secure
	if !util.IsSecureFromSlack(context) {
		log.Println("Insecure request skipped.")
		return
	}

	// Parse the event
	var wrapper eventWrapper
	parseError := context.BindJSON(&wrapper)
	if util.InternalError(parseError, context) {
		return
	}

	// If this is a challenge request, respond
	if wrapper.Type == "url_verification" {
		context.String(http.StatusOK, wrapper.Challenge)
		return
	}

	// Extract the inner event
	event := wrapper.Event

	if util.InternalError(database.EnsureUserExists(event.User), context) {
		return
	}

	// If type is a app_home_opened, answer it
	if event.Type == "app_home_opened" {
		// Check if spotify has been connected yet for this session
		profileID, _, dbError := database.GetSpotifyForUser(event.User)
		if util.InternalError(dbError, context) {
			return
		}

		log.Println("Spotify for user:", profileID)

		if profileID == "" { // Serve a new welcome screen
			pageError := slack.CreateNewUserHomepage(event.User, client)
			if util.InternalError(pageError, context) {
				return
			}
		} else { // Serve an all-set screen
			pageError := slack.CreateReturningHomepage(event.User, client)
			if util.InternalError(pageError, context) {
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
