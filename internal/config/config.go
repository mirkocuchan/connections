package config

import (
	"os"
	"errors"
	"github.com/joho/godotenv"
	"log"
	"time"
)

type Config struct {
	DBURL string
	PORT string
	JWTSecret string
	AccessTokenDuration time.Duration
	RefreshTokenDuration time.Duration
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

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return Config{}, errors.New("JWT_SECRET environment variable is not set")
	}
	accessTokenDuration, accessErr := time.ParseDuration(os.Getenv("ACCESS_TOKEN_DURATION"))
	refreshTokenDuration, refreshErr := time.ParseDuration(os.Getenv("REFRESH_TOKEN_DURATION"))
	if accessErr != nil{
		return Config{}, errors.New("ACCESS_TOKEN_DURATION environment variable is not set")
	}
	if refreshErr != nil{
		return Config{}, errors.New("REFRESH_TOKEN_DURATION environment variable is not set")
	}

	config := Config{
		DBURL: dbURL,
		PORT: port,
		JWTSecret: jwtSecret,
		AccessTokenDuration: accessTokenDuration,
		RefreshTokenDuration: refreshTokenDuration,
	}
	return config, nil
}