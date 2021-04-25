package database

import (
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

// Rolls back the transaction and returns the causing error. If rollback fails, returns that error instead.
func rollbackOnError(transaction *sqlx.Tx, err error) error {
	rollbackError := transaction.Rollback()
	if rollbackError != nil {
		return rollbackError
	}
	return err
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

func updateRow(transaction *sqlx.Tx, mustEffect bool, query string, params ...interface{}) error {
	// If transaction is given, use it. If not, use the DB pool
	var results sql.Result
	var rowUpdateError error
	if transaction != nil {
		results, rowUpdateError = transaction.Exec(query, params...)
	} else {
		results, rowUpdateError = appDatabase.Exec(query, params...)
	}

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
