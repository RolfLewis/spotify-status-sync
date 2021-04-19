package spotify

import (
	"net/http"

	"rolflewis.com/spotify-status-sync/src/database"
)

func RefreshExpiringTokens(client *http.Client) (int, error) {
	// Get the list of users who need to be refreshed
	users, usersError := database.GetAllUsersWhoExpireWithinXMinutes(20)
	if usersError != nil {
		return 0, usersError
	}
	// Refresh each user
	for index, user := range users {
		refreshError := refreshTokenForUser(user, client)
		if refreshError != nil {
			return index, refreshError
		}
	}
	// Return success
	return len(users), nil
}

func refreshTokenForUser(user string, client *http.Client) error {
	// Get the spotify token data for the user
	spotifyID, oldTokens, spotifyError := database.GetSpotifyForUser(user)
	if spotifyError != nil {
		return spotifyError
	}

	// Exchange code for tokens
	tokensMap, exchangeError := ExchangeCodeForTokens(oldTokens[1], true, client)
	if exchangeError != nil {
		return exchangeError
	}

	// If no new refresh token was given, save the old one
	_, exists := tokensMap["refresh_token"]
	if !exists {
		tokensMap["refresh_token"] = oldTokens[1]
	}

	// Insert new tokens into databse
	return database.AddSpotifyToUser(user, spotifyID, tokensMap["access_token"].(string), tokensMap["refresh_token"].(string), int(tokensMap["expires_in"].(float64)))
}
