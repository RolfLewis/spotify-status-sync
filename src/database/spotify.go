package database

import (
	"database/sql"
	"time"
)

func DeleteSpotifyDataForUser(user string) error {
	// Get the spotify account id for the user
	var spotifyID string
	scanError := appDatabase.QueryRowx("SELECT spotify_id FROM slackaccounts WHERE id=$1 AND spotify_id IS NOT null;", user).Scan(&spotifyID)
	if scanError != nil && scanError != sql.ErrNoRows {
		return scanError
	}
	// Remove the spotify record key from the slackaccount record first if exists
	updateError := updateRow(nil, false, "UPDATE slackaccounts SET spotify_id=null WHERE id=$1;", user)
	if updateError != nil {
		return updateError
	}
	// Delete the spotify record
	if spotifyID != "" {
		_, spotifyDeleteError := appDatabase.Exec("DELETE FROM spotifyaccounts WHERE id=$1;", spotifyID)
		return spotifyDeleteError
	}
	// return success
	return nil
}

func GetAllUsersWhoExpireWithinXMinutes(minutes int) ([]string, error) {
	// Calculate the expiration timeframe
	cutoff := time.Now().Add(time.Minute * time.Duration(minutes))

	// Get user id where spotify expires in less than x minutes
	var users []string
	selectError := appDatabase.Select(&users, "SELECT slackaccounts.id FROM slackaccounts LEFT JOIN spotifyaccounts on slackaccounts.spotify_id = spotifyaccounts.id WHERE spotifyaccounts.expirationAt <= $1;", cutoff)
	if selectError != nil {
		return nil, selectError
	}

	// Return the list of users
	return users, nil
}
