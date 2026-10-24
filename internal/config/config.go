package config

import (
	"os"
	"errors"
	"github.com/joho/godotenv"
	"log"
	"time"
	"fmt"
	"strings"
	"strconv"
)

type Config struct {
	DBURL string
	PORT string
	JWTSecret string
	AccessTokenDuration time.Duration
	RefreshTokenDuration time.Duration
	UploadDir string
	TrustProxy bool
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
	// las plataformas de hosting dan solo el número ("8080"), ListenAndServe necesita ":8080"
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return Config{}, errors.New("JWT_SECRET environment variable is not set")
	}
	if len(jwtSecret) < 32 {
		return Config{}, errors.New("JWT_SECRET must be at least 32 characters")
	}
	accessTokenDuration, accessErr := time.ParseDuration(os.Getenv("ACCESS_TOKEN_DURATION"))
	refreshTokenDuration, refreshErr := time.ParseDuration(os.Getenv("REFRESH_TOKEN_DURATION"))
		if accessErr != nil {
		return Config{}, fmt.Errorf("ACCESS_TOKEN_DURATION is missing or invalid (example: 15m): %w", accessErr)
	}
	if refreshErr != nil {
		return Config{}, fmt.Errorf("REFRESH_TOKEN_DURATION is missing or invalid (example: 720h): %w", refreshErr)
	}
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "uploads" //valor por defecto, con esto: `go run` sigue andando como siempre
	}

	trustProxy := false //por defecto NO confiamos en headers de proxy
	if v := os.Getenv("TRUST_PROXY"); v != "" {
		parsed, perr := strconv.ParseBool(v)
		if perr != nil {
			return Config{}, fmt.Errorf("TRUST_PROXY must be true or false: %w", perr)
		}
		trustProxy = parsed
	}

	config := Config{
		DBURL: dbURL,
		PORT: port,
		JWTSecret: jwtSecret,
		AccessTokenDuration: accessTokenDuration,
		RefreshTokenDuration: refreshTokenDuration,
		UploadDir: uploadDir,
		TrustProxy: trustProxy,
	}
	return config, nil
}