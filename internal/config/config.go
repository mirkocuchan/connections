package config

import (
	"os"
	"errors"
)

type Config struct {
	DBURL string
}

func GetConfig() (Config, error){
	//getting the dbURL, in this case from the .env to open the connection
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return Config{}, errors.New("error reading .env")
	}

	newConfig := Config{
		DBURL: dbURL,
	}
	return newConfig, nil
}