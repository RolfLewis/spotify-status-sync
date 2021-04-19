package database

import (
	"database/sql"
	"errors"
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
	updateError := updateRow(true, "UPDATE slackaccounts SET spotify_id=$1 WHERE id=$2;", id, user)
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
	return updateRow(true, "UPDATE slackaccounts SET accesstoken=$1 WHERE id=$2;", token, user)
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
	token, getError := getSingleString("SELECT accessToken FROM slackaccounts WHERE id=$1 AND accessToken IS NOT null;", user)
	return token, getError
}

func GetStatusForUser(user string) (string, error) {
	// Get the status string for the user
	status, getError := getSingleString("SELECT status FROM slackaccounts WHERE id=$1 AND status IS NOT null;", user)
	return status, getError
}

func getSingleString(query string, params ...interface{}) (string, error) {
	var object interface{}
	getError := appDatabase.Get(&object, query, params...)
	if getError == sql.ErrNoRows {
		return "", nil
	} else if getError != nil {
		return "", getError
	}
	return object.(string), nil
}

func SetStatusForUser(user string, status string) error {
	// Update this record
	return updateRow(true, "UPDATE slackaccounts SET status=$1 WHERE id=$2;", status, user)
}

func updateRow(mustEffect bool, query string, params ...interface{}) error {
	// Update the row
	results, rowUpdateError := appDatabase.Exec(query, params...)
	if rowUpdateError != nil {
		return rowUpdateError
	}
	// If mustEffect, we throw an error when no row was affected
	if mustEffect {
		// Check to make sure a row was found
		rowsAffected, affectedError := results.RowsAffected()
		if affectedError != nil {
			return affectedError
		}
		// If no rows were overwritten, then nothing had that ID
		if rowsAffected == 0 {
			return errors.New("No row was modified during update, and mustEffect is set to true.")
		}
	}
	// return success
	return nil
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
	// No spotify data to delete
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
