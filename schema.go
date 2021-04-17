package main

import (
	"database/sql"
	"log"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
)

var appDatabase *sqlx.DB

var tables = []string{
	"sessions",
	"spotifyAccounts",
	// "slackAccounts",
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

	createTableIfNotExists("spotifyAccounts", `CREATE TABLE spotifyAccounts (id text CONSTRAINT spotify_pk PRIMARY KEY NOT null,
		accessToken text, refreshToken text, expirationAt timestamp);`)

	createTableIfNotExists("slackAccounts", `CREATE TABLE slackAccounts (id uuid CONSTRAINT slack_pk PRIMARY KEY NOT null,
		accessToken text, refreshToken text, expirationAt timestamp,
		spotify_id text, CONSTRAINT spotify_fk FOREIGN KEY(spotify_id) REFERENCES spotifyAccounts(id));`)

	createTableIfNotExists("sessions", `CREATE TABLE sessions (session_id uuid CONSTRAINT session_pk PRIMARY KEY NOT null,
		spotify_id text, CONSTRAINT spotify_fk FOREIGN KEY(spotify_id) REFERENCES spotifyAccounts(id),
		slack_id uuid, CONSTRAINT slack_fk FOREIGN KEY(slack_id) REFERENCES slackAccounts(id));`)
}

func addNewSession(id string) error {
	_, rowInsertError := appDatabase.Exec("INSERT INTO sessions VALUES ($1);", id)
	return rowInsertError
}

func addSpotifyToSession(session string, profile spotifyProfile, tokens spotifyAuthResponse) error {
	expirationTime := time.Now().Add(time.Second * time.Duration(tokens.ExpiresIn))
	_, rowInsertError := appDatabase.Exec("INSERT INTO spotifyAccounts VALUES ($1, $2, $3, $4);", profile.ID, tokens.AccessToken, tokens.RefreshToken, expirationTime)
	if rowInsertError != nil {
		return rowInsertError
	}

	_, rowUpdateError := appDatabase.Exec("UPDATE spotifyAccounts SET spotify_id=$1 WHERE session_id=$2", profile.ID, session)
	return rowUpdateError
}

func getSpotifyForSession(session string) (string, *spotifyAuthResponse, error) {
	// Get the spotify ID from the session
	var spotifyID string
	scanError := appDatabase.QueryRowx("SELECT spotify_id FROM sessions WHERE session_id=$1", session).Scan(&spotifyID)
	if scanError != nil && scanError == sql.ErrNoRows {
		return "", nil, nil // Nothing to return
	} else if scanError != nil {
		return "", nil, scanError
	}
	// Get the spotify tokens
	fields, tokensScanError := appDatabase.QueryRowx("SELECT accessToken, refreshToken FROM spotifyAccounts WHERE id=$1", spotifyID).SliceScan()
	if tokensScanError != nil { // This row must exist because of the FK relationship so we don't need to test for row count
		return "", nil, tokensScanError
	}
	// Read the tokens into an object and return
	return spotifyID, &spotifyAuthResponse{
		AccessToken:  fields[0].(string),
		RefreshToken: fields[1].(string),
	}, nil
}
