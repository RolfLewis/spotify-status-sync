package routes

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"rolflewis.com/spotify-status-sync/src/database"
	"rolflewis.com/spotify-status-sync/src/slack"
	"rolflewis.com/spotify-status-sync/src/util"
)

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

func InteractivityEndpoint(context *gin.Context, client *http.Client) {
	// Ensure is from slack and is secure
	if !util.IsSecureFromSlack(context) {
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
	if util.InternalError(headerParseError, context) {
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
	if util.InternalError(interactionParseError, context) {
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
			// Delete spotify data
			deleteError := database.DeleteSpotifyDataForUser(interaction.User.ID)
			if util.InternalError(deleteError, context) {
				return
			}
			// After removing the data, reset the user's app home view back to the new user flow
			viewError := slack.UpdateHome(interaction.User.ID, client)
			if util.InternalError(viewError, context) {
				return
			}
		}
	}

	// Return an interaction success
	context.String(http.StatusOK, "Interaction Processed.")
}
