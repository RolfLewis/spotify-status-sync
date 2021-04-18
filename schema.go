package main

import (
	"database/sql"
	"log"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var appDatabase *sqlx.DB

var tables = []string{
	"spotifyaccounts",
	"slackaccounts",
}

func connectToDatabase() {
	database, dbError := sqlx.Connect("postgres", os.Getenv("DATABASE_URL"))
	if dbError != nil {
		log.Panic(dbError)
	}
	// Performance Settings
	database.SetConnMaxLifetime(0)
	database.SetMaxOpenConns(10)
	appDatabase = database
}

func disconnectDatabase() {
	dbError := appDatabase.Close()
	if dbError != nil {
		log.Println(dbError)
	}
}

func nukeDatabase() {
	for _, table := range tables {
		appDatabase.Exec("DROP TABLE " + table + " CASCADE;")
	}
}

func validateSchema() {
	createTableIfNotExists := func(tableParam string, createCmd string) {
		// Check if the table exists
		var tableName string
		getError := appDatabase.Get(&tableName, "select table_name from information_schema.tables where table_name=$1", tableParam)
		if getError != nil && getError != sql.ErrNoRows {
			log.Panic(getError)
		}

		if getError != nil {
			_, tableCreateError := appDatabase.Exec(createCmd)
			if tableCreateError != nil {
				log.Println("Error when creating", tableParam, ":", createCmd)
				log.Panic(tableCreateError)
			}
		}
	}

	createTableIfNotExists("spotifyaccounts", `CREATE TABLE spotifyaccounts (id text CONSTRAINT spotify_pk PRIMARY KEY NOT null,
		accessToken text, refreshToken text, expirationAt timestamp);`)

	createTableIfNotExists("slackaccounts", `CREATE TABLE slackaccounts (id text CONSTRAINT slack_pk PRIMARY KEY NOT null,
		accessToken text, refreshToken text, expirationAt timestamp,
		spotify_id text, CONSTRAINT spotify_fk FOREIGN KEY(spotify_id) REFERENCES spotifyaccounts(id));`)
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

func addSpotifyToUser(user string, profile spotifyProfile, tokens spotifyAuthResponse) error {
	expirationTime := time.Now().Add(time.Second * time.Duration(tokens.ExpiresIn))
	_, rowUpsertError := appDatabase.Exec("INSERT INTO spotifyaccounts VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO UPDATE SET accessToken=$2, refreshToken=$3, expirationAt=$4;", profile.ID, tokens.AccessToken, tokens.RefreshToken, expirationTime)
	if rowUpsertError != nil {
		return rowUpsertError
	}

	_, rowUpdateError := appDatabase.Exec("UPDATE slackaccounts SET spotify_id=$1 WHERE id=$2", profile.ID, user)
	return rowUpdateError
}

func getSpotifyForUser(user string) (string, *spotifyAuthResponse, error) {
	// Get the spotify ID from the session
	var spotifyID string
	scanError := appDatabase.QueryRowx("SELECT spotify_id FROM slackaccounts WHERE id=$1 AND spotify_id NOT null", user).Scan(&spotifyID)
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
	return spotifyID, &spotifyAuthResponse{
		AccessToken:  fields[0].(string),
		RefreshToken: fields[1].(string),
	}, nil
}
