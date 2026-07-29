package config

import (
	"os"
	"errors"
	"github.com/joho/godotenv"
	"log"
)

type Config struct {
	DBURL string
	PORT string
}

func GetConfig() (Config, error){
	//cargamos el .env para despues poder leerlo, en caso de que no exista, seguimos
	err := godotenv.Load()
	if err != nil{
		log.Println("warning: .env file not found or couldn't be loaded, reading from environment variables")
	}
	
	//getting the dbURL from the environment
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return Config{}, errors.New("DB_URL environment variable is not set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		return Config{}, errors.New("PORT environment variable is not set")
	}

	config := Config{
		DBURL: dbURL,
		PORT: port,
	}
	return config, nil
}