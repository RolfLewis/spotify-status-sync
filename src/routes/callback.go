package routes

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"rolflewis.com/spotify-status-sync/src/database"
	"rolflewis.com/spotify-status-sync/src/slack"
	"rolflewis.com/spotify-status-sync/src/spotify"
	"rolflewis.com/spotify-status-sync/src/util"
)

type SpotifyAuthResponse struct {
	AccessToken  string
	ExpiresIn    int
	RefreshToken string
}

func SlackCallbackFlow(context *gin.Context, client *http.Client) {
	// Read the auth code
	code := context.Query("code")

	// Read the user id (passed as state)
	user := context.Query("state")

	// if no state is somehow defined, bad request
	if user == "" {
		log.Println("No state/userid defined in callback request.")
		context.String(http.StatusBadRequest, "No state/userid defined in callback request.")
	}

	// Exchange code for token
	authResponse, exchangeError := slack.ExchangeCodeForToken(code, client)
	if util.InternalError(exchangeError, context) {
		return
	}

	// Make sure the team exists in DB
	teamExistsError := database.EnsureTeamExists(authResponse.Team.ID)
	if util.InternalError(teamExistsError, context) {
		return
	}

	// Set token for team
	teamUpdateError := database.SetTokenForTeam(authResponse.Team.ID, authResponse.AccessToken)
	if util.InternalError(teamUpdateError, context) {
		return
	}

	// Make sure we have a user record for the user
	if util.InternalError(database.EnsureUserExists(user), context) {
		return
	}

	// Set the user's team id
	if util.InternalError(database.SetTeamForUser(user, authResponse.Team.ID), context) {
		return
	}

	// Save to user record
	saveError := database.SaveSlackTokenForUser(user, authResponse.AuthedUser.AccessToken)
	if util.InternalError(saveError, context) {
		return
	}

	// update the homepage view
	viewError := slack.UpdateHome(user, client)
	if util.InternalError(viewError, context) {
		return
	}

	context.String(http.StatusOK, "Signed in. You can close this window now.")
}

func SpotifyCallbackFlow(context *gin.Context, client *http.Client) {
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
	if util.InternalError(database.EnsureUserExists(user), context) {
		return
	}

	// Exchange code for tokens
	tokensMap, exchangeError := spotify.ExchangeCodeForTokens(code, false, client)
	if util.InternalError(exchangeError, context) {
		return
	}

	// Convert the map of tokens into a struct
	tokens := SpotifyAuthResponse{
		AccessToken:  tokensMap["access_token"].(string),
		RefreshToken: tokensMap["refresh_token"].(string),
		ExpiresIn:    int(tokensMap["expires_in"].(float64)), // we don't care about the fractional here because of how we will handle refreshes
	}

	// Get the user's profile information
	profile, profileError := spotify.GetProfileForTokens(tokens.AccessToken, client)
	if util.InternalError(profileError, context) {
		return
	}

	// Save the information to the DB
	dbError := database.AddSpotifyToUser(user, *profile, tokens.AccessToken, tokens.RefreshToken, tokens.ExpiresIn)
	if util.InternalError(dbError, context) {
		return
	}

	// update the homepage view
	viewError := slack.UpdateHome(user, client)
	if util.InternalError(viewError, context) {
		return
	}

	context.String(http.StatusOK, "Signed in. You can close this window now.")
}
