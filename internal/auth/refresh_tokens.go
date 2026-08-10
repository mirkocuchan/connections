package main

import(
	"crypto/rand"
	"encoding/hex"
	"errors"
	"crypto/sha256"
)

func MakeRefreshToken() (string, error){
	randomString := make([]byte, 32)
	ans, err := rand.Read(randomString)
	if err != nil{
		return "", errors.New("error filling the array with random bytes")
	}
	return hex.EncodeToString(randomString), nil
} 
//why do we need to make a refresh token? because we don't want to store the user's password in the database. we want to store a refresh token that is generated randomly and is unique for each user. 
//this way, we can authenticate the user without having to store their password. how does make([]byte, 32) work? it creates a slice of bytes with a length of 32. rand.Read fills the slice with random bytes. hex.EncodeToString converts the slice of bytes to a string of hexadecimal characters.

//hasheamos el refresh token que viene en forma de string
func HashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
//how does this work? sha256.Sum256 takes a slice of bytes and returns a [32]byte array. we convert the string to a slice of bytes using []byte(token). 
//then we convert the [32]byte array to a slice of bytes using hash[:]. finally, we convert the slice of bytes to a string of hexadecimal characters using hex.EncodeToString.

func GetBearerToken(headers http.Header) (string, error){
	authorizationHeader := headers.Get("Authorization")
	if strings.HasPrefix(authorizationHeader, "Bearer "){
		return strings.TrimPrefix(authorizationHeader, "Bearer "), nil
	}
	return "", errors.New("No Authorization Header")
	
}
