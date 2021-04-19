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

func CallbackFlow(context *gin.Context, client *http.Client) {
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
	viewError := slack.CreateReturningHomepage(user, client)
	if util.InternalError(viewError, context) {
		return
	}

	context.String(http.StatusOK, "Signed in. You can close this window now.")
}
