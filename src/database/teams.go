package database

func addNewTeam(team string) error {
	_, rowInsertError := appDatabase.Exec("INSERT INTO teams VALUES ($1);", team)
	return rowInsertError
}

func teamExists(team string) (bool, error) {
	// Get the team
	result, getError := getSingleString("SELECT id FROM team WHERE id=$1;", team)
	return (result != ""), getError
}

func EnsureTeamExists(team string) error {
	// Make sure that a team record exists for the id
	exists, existsError := teamExists(team)
	if existsError != nil {
		return existsError
	}

	// Create a team record if needed
	if !exists {
		teamAddError := addNewTeam(team)
		if teamAddError != nil {
			return teamAddError
		}
	}

	return nil
}

func SetTokenForTeam(team string, token string) error {
	return updateRow(nil, true, "UPDATE teams SET accesstoken=$1 WHERE id=$2;", token, team)
}
