package database

import (
	"database/sql"
	"errors"
	"time"
)

type SpotifyAuthResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type SpotifyProfile struct {
	ID string `json:"id"`
}

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
func AddSpotifyToUser(user string, profile SpotifyProfile, tokens SpotifyAuthResponse) error {
	// Open a transaction on the DB - roll it back if anything fails
	transaction, transactionError := appDatabase.Beginx()
	if transactionError != nil {
		return rollbackOnError(transaction, transactionError)
	}

	// Insert the new spotify record
	expirationTime := time.Now().Add(time.Second * time.Duration(tokens.ExpiresIn))
	_, rowUpsertError := transaction.Exec("INSERT INTO spotifyaccounts VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO UPDATE SET accessToken=$2, refreshToken=$3, expirationAt=$4;", profile.ID, tokens.AccessToken, tokens.RefreshToken, expirationTime)
	if rowUpsertError != nil {
		return rollbackOnError(transaction, rowUpsertError)
	}

	// Tie the slack account to the spotify user
	results, rowUpdateError := transaction.Exec("UPDATE slackaccounts SET spotify_id=$1 WHERE id=$2", profile.ID, user)
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

func GetSpotifyForUser(user string) (string, *SpotifyAuthResponse, error) {
	// Get the spotify ID from the session
	var spotifyID string
	scanError := appDatabase.QueryRowx("SELECT spotify_id FROM slackaccounts WHERE id=$1 AND spotify_id IS NOT null", user).Scan(&spotifyID)
	if scanError == sql.ErrNoRows {
		return "", nil, nil // Nothing to return
	} else if scanError != nil {
		return "", nil, scanError
	}
	// Get the spotify tokens
	fields, tokensScanError := appDatabase.QueryRowx("SELECT accessToken, refreshToken FROM spotifyaccounts WHERE id=$1", spotifyID).SliceScan()
	if tokensScanError != nil { // This row must exist because of the FK relationship so we don't need to test for row count
		return "", nil, tokensScanError
	}
	// Read the tokens into an object and return
	return spotifyID, &SpotifyAuthResponse{
		AccessToken:  fields[0].(string),
		RefreshToken: fields[1].(string),
	}, nil
}

func DeleteAllDataForUser(user string) error {
	// Get the spotify account id for the user
	var spotifyID string
	scanError := appDatabase.QueryRowx("SELECT spotify_id FROM slackaccounts WHERE id=$1 AND spotify_id IS NOT null", user).Scan(&spotifyID)
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
		_, spotifyDeleteError := appDatabase.Exec("DELETE FROM spotifyaccounts WHERE id=$1", spotifyID)
		return spotifyDeleteError
	}
	// No spotify data to delete
	return nil
}