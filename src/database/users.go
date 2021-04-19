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
	var id string
	scanError := appDatabase.QueryRowx("SELECT id FROM slackaccounts WHERE id=$1", user).Scan(&id)
	if scanError == sql.ErrNoRows {
		return false, nil
	} else if scanError != nil {
		return false, scanError
	}
	return true, nil
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
	results, rowUpdateError := transaction.Exec("UPDATE slackaccounts SET spotify_id=$1 WHERE id=$2;", id, user)
	if rowUpdateError != nil {
		return rollbackOnError(transaction, rowUpdateError)
	}

	// Check to make sure a row was found
	rowsAffected, affectedError := results.RowsAffected()
	if affectedError != nil {
		return rollbackOnError(transaction, affectedError)
	}

	// If no rows were overwritten, then nothing had that ID
	if rowsAffected == 0 {
		return rollbackOnError(transaction, errors.New("No slack account record exists with this user id"))
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
	// Set the access token on the user record
	results, rowUpdateError := appDatabase.Exec("UPDATE slackaccounts SET accesstoken=$1 WHERE id=$2;", token, user)
	if rowUpdateError != nil {
		return rowUpdateError
	}
	// Check to make sure a row was found
	rowsAffected, affectedError := results.RowsAffected()
	if affectedError != nil {
		return affectedError
	}
	// If no rows were overwritten, then nothing had that ID
	if rowsAffected == 0 {
		return errors.New("No slack account record exists with this user id")
	}
	// return success
	return nil
}

func GetSpotifyForUser(user string) (string, []string, error) {
	// Get the spotify ID from the user
	var spotifyID string
	getError := appDatabase.Get(&spotifyID, "SELECT spotify_id FROM slackaccounts WHERE id=$1 AND spotify_id IS NOT null;", user)
	if getError == sql.ErrNoRows {
		return "", nil, nil // Nothing to return
	} else if getError != nil {
		return "", nil, getError
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
	var token string
	getError := appDatabase.Get(&token, "SELECT accessToken FROM slackaccounts WHERE id=$1 AND accessToken IS NOT null;", user)
	if getError == sql.ErrNoRows {
		return "", nil // Nothing to return
	} else if getError != nil {
		return "", getError
	}
	return token, nil
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
