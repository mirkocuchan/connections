package auth

import(
	"errors"
	"github.com/alexedwards/argon2id"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)
//hasheamos la contraseña que viene en forma de string
func HashPassword(password string) (string, error){
	hashedPassword, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", errors.New("couldn't hash the password")
	}
	return hashedPassword, nil
}
//comparamos la password que viene en formato de string de la app con el hash que tenemos guardado
//si no es igual, devolvemos false y que no matchea
func CheckPasswordHash(password, hash string) (bool, error){
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return match, errors.New("password does not match the password in the database")
	}
	return match, nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error){
	newJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		Issuer: "connections-access",
		Subject: userID.String(),
		IssuedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(expiresIn).Unix(), //why add unix at the end? because jwt expects the exp claim to be a unix timestamp, not a time.Time object. so we need to convert it to a unix timestamp using .Unix()
	})
	signedJWT, err := newJWT.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", errors.New("couldn't sign the JWT")
	}
	return signedJWT, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error){
	claims := jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(*Token) (interface{}, error))
	if err != nil {
		return uuid.Nil, errors.New("couldn't parse the JWT")
	}
	

}


func MakeRefreshToken() string{
	randomString := make([]byte, 32)
	rand.Read(randomString)
	return hex.EncodeToString(randomString)
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