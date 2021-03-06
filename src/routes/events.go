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
	Tokens    struct {
		OAuth []string `json:"oauth"`
		Bot   []string `json:"bot"`
	} `json:"tokens"`
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
	} else if wrapper.Type == "event_callback" {
		// Extract the inner event
		event := wrapper.Event

		// If type is a app_home_opened, answer it
		if event.Type == "app_home_opened" {
			// Make sure that this user exists
			if util.InternalError(database.EnsureUserExists(event.User), context) {
				return
			}
			// Make sure the team exists in DB
			teamExistsError := database.EnsureTeamExists(wrapper.TeamID)
			if util.InternalError(teamExistsError, context) {
				return
			}
			// Set the user's team id
			if util.InternalError(database.SetTeamForUser(event.User, wrapper.TeamID), context) {
				return
			}
			// Update the home page
			updateError := slack.UpdateHome(event.User, client)
			if util.InternalError(updateError, context) {
				return
			}
			// Send an acknowledgment
			context.String(http.StatusOK, "Ok")
		} else if event.Type == "tokens_revoked" {
			// Delete all of the users related to revoked user tokens
			for _, user := range event.Tokens.OAuth {
				// Clean out the spotify and slack authorization data so a page update is essentially like new
				spotifyClearError := database.DeleteSpotifyDataForUser(user)
				if util.InternalError(spotifyClearError, context) {
					return
				}
				slackTokenClear := database.SaveSlackTokenForUser(user, "")
				if util.InternalError(slackTokenClear, context) {
					return
				}
				// Delete user data
				log.Println("Cleaning up former user.")
				cleanupError := database.DeleteAllDataForUser(user)
				if util.InternalError(cleanupError, context) {
					return
				}
			}
			// Delete the team data and token of revoked bot tokens
			if len(event.Tokens.Bot) > 0 {
				// Delete the team record
				teamDeleteError := database.DeleteAllDataForTeam(wrapper.TeamID)
				if util.InternalError(teamDeleteError, context) {
					return
				}
			}
		} else {
			context.String(http.StatusBadRequest, "Not a supported event")
			log.Println("Not a supported event:", event)
		}
	} else {
		context.String(http.StatusBadRequest, "Not a supported event")
		log.Println("Not a supported event:", wrapper)
	}
}
