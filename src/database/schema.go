package database

import (
	"database/sql"
	"log"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var appDatabase *sqlx.DB

func ConnectToDatabase() {
	database, dbError := sqlx.Connect("postgres", os.Getenv("DATABASE_URL"))
	if dbError != nil {
		log.Panic(dbError)
	}
	// Performance Settings
	database.SetConnMaxLifetime(0)
	database.SetMaxOpenConns(10)
	appDatabase = database
}

func DisconnectDatabase() {
	dbError := appDatabase.Close()
	if dbError != nil {
		log.Println(dbError)
	}
}

// Rolls back the transaction and returns the causing error. If rollback fails, returns that error instead.
func rollbackOnError(transaction *sqlx.Tx, err error) error {
	rollbackError := transaction.Rollback()
	if rollbackError != nil {
		return rollbackError
	}
	return err
}

func ValidateSchema() {
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

	// Stores information related to the bot user in each team - saved during callback
	createTableIfNotExists("teams", `CREATE TABLE teams (id text CONSTRAINT team_pk PRIMARY KEY NOT null, accesstoken text);`)

	// Stores information for each connected spotify profile
	createTableIfNotExists("spotifyaccounts", `CREATE TABLE spotifyaccounts (id text CONSTRAINT spotify_pk PRIMARY KEY NOT null,
		accesstoken text, refreshtoken text, expirationat timestamp);`)

	// Stores information related to each slack user of the app
	createTableIfNotExists("slackaccounts", `CREATE TABLE slackaccounts (id text CONSTRAINT slack_pk PRIMARY KEY NOT null,
		status text, accesstoken text, spotify_id text, team_id text,
		CONSTRAINT spotify_fk FOREIGN KEY(spotify_id) REFERENCES spotifyaccounts(id),
		CONSTRAINT team_fk FOREIGN KEY(team_id) REFERENCES teams(id));`)
}
