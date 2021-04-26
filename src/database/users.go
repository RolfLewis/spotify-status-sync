package database

import (
	"database/sql"
	"time"
)

func addNewUser(user string) error {
	_, rowInsertError := appDatabase.Exec("INSERT INTO slackaccounts VALUES ($1);", user)
	return rowInsertError
}

func userExists(user string) (bool, error) {
	// Get the user
	result, getError := getSingleString("SELECT id FROM slackaccounts WHERE id=$1", user)
	return (result != ""), getError
}

func GetAllConnectedUsers() ([]string, error) {
	// Get all user ids where they have a connected spotify account
	var users []string
	selectError := appDatabase.Select(&users, "SELECT id FROM slackaccounts WHERE accesstoken IS NOT null AND spotify_id IS NOT null;")
	return users, selectError
}

func EnsureUserExists(user string) error {
	// Make sure that a user record exists for the user
	exists, existsError := userExists(user)
	if existsError != nil {
		return existsError
	}

	// Create a user record if needed
	if !exists {
		userAddError := addNewUser(user)
		if userAddError != nil {
			return userAddError
		}
	}

	return nil
}

func SetTeamForUser(user string, team string) error {
	return updateRow(nil, true, "UPDATE slackaccounts SET team_id=$1 WHERE id=$2;", team, user)
}

// Adds the spotify information to the DB using a transaction. Rolls back on any error. Returns rollback error if one occurs.
func AddSpotifyToUser(user string, id string, accessToken string, refreshToken string, expiresIn int) error {
	// Open a transaction on the DB - roll it back if anything fails
	transaction, transactionError := appDatabase.Beginx()
	if transactionError != nil {
		return rollbackOnError(transaction, transactionError)
	}

	// Insert the new spotify record
	expirationTime := time.Now().Add(time.Second * time.Duration(expiresIn))
	_, rowUpsertError := transaction.Exec("INSERT INTO spotifyaccounts VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO UPDATE SET accessToken=$2, refreshToken=$3, expirationAt=$4;", id, accessToken, refreshToken, expirationTime)
	if rowUpsertError != nil {
		return rollbackOnError(transaction, rowUpsertError)
	}

	// Tie the slack account to the spotify user
	updateError := updateRow(transaction, true, "UPDATE slackaccounts SET spotify_id=$1 WHERE id=$2;", id, user)
	if updateError != nil {
		return rollbackOnError(transaction, updateError)
	}

	// Commit the transaction to the DB
	commitError := transaction.Commit()
	if commitError != nil {
		return rollbackOnError(transaction, commitError)
	}

	// Return success
	return nil
}

func SaveSlackTokenForUser(user string, token string) error {
	// Update this record
	return updateRow(nil, true, "UPDATE slackaccounts SET accesstoken=$1 WHERE id=$2;", token, user)
}

func GetSpotifyForUser(user string) (string, []string, error) {
	// Get the spotify ID from the user
	spotifyID, getError := getSingleString("SELECT spotify_id FROM slackaccounts WHERE id=$1 AND spotify_id IS NOT null;", user)
	if getError != nil {
		return "", nil, getError
	}

	// If spotify not connected, return blank data
	if spotifyID == "" {
		return "", nil, nil
	}

	// Get the spotify tokens
	fields, tokensScanError := appDatabase.QueryRowx("SELECT accessToken, refreshToken FROM spotifyaccounts WHERE id=$1;", spotifyID).SliceScan()
	if tokensScanError != nil { // This row must exist because of the FK relationship so we don't need to test for row count
		return "", nil, tokensScanError
	}

	// Convert interface array to strings array
	tokens := []string{
		fields[0].(string),
		fields[1].(string),
	}

	// Read the tokens into an object and return
	return spotifyID, tokens, nil
}

func GetSlackForUser(user string) (string, error) {
	// Get the token for the user
	return getSingleString("SELECT accessToken FROM slackaccounts WHERE id=$1 AND accessToken IS NOT null;", user)
}

func GetStatusForUser(user string) (string, error) {
	// Get the status string for the user
	return getSingleString("SELECT status FROM slackaccounts WHERE id=$1 AND status IS NOT null;", user)
}

func GetTeamTokenForUser(user string) (string, error) {
	return getSingleString("SELECT teams.accesstoken FROM slackaccounts LEFT JOIN teams ON slackaccounts.team_id = teams.id WHERE slackaccounts.id=$1 AND teams.accesstoken IS NOT null;", user)
}

func SetStatusForUser(user string, status string) error {
	// Update this record
	return updateRow(nil, true, "UPDATE slackaccounts SET status=$1 WHERE id=$2;", status, user)
}

func DeleteAllDataForUser(user string) error {
	// Get the spotify account id for the user
	var spotifyID string
	scanError := appDatabase.QueryRowx("SELECT spotify_id FROM slackaccounts WHERE id=$1 AND spotify_id IS NOT null;", user).Scan(&spotifyID)
	if scanError != nil && scanError != sql.ErrNoRows {
		return scanError
	}
	// Delete the slack account record
	_, slackDeleteError := appDatabase.Exec("DELETE FROM slackaccounts WHERE id=$1;", user)
	if slackDeleteError != nil {
		return slackDeleteError
	}
	// Delete the spotify record
	if spotifyID != "" {
		_, spotifyDeleteError := appDatabase.Exec("DELETE FROM spotifyaccounts WHERE id=$1;", spotifyID)
		return spotifyDeleteError
	}
	// Delete spotify data
	return nil
}

func GetUsersForTeam(team string) ([]string, error) {
	// Get all the user ids related to the given team id
	var users []string
	selectError := appDatabase.Select(&users, "SELECT id FROM slackaccounts WHERE team_id=$1;", team)
	return users, selectError
}
