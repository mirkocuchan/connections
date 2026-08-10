package auth
import(
	"errors"
	"github.com/alexedwards/argon2id"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
	"github.com/google/uuid"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"strings"
)

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error){
	issuer := "connections-access"
	subject := userID.String()
	issuedAt := time.Now().Unix()
	expiresAt := time.Now().Add(expiresIn).Unix() //why add unix at the end? because jwt expects the exp claim to be a unix timestamp, not a time.Time object. so we need to convert it to a unix timestamp using .Unix()

	claims := jwt.RegisteredClaims{
		Issuer:  issuer,
		Subject: subject,
		IssuedAt: jwt.NewNumericDate(time.Unix(issuedAt, 0)),
		ExpiresAt: jwt.NewNumericDate(time.Unix(expiresAt, 0)),
	}
	//why use newnumericdate? because jwt.RegisteredClaims expects the IssuedAt and ExpiresAt fields to be of type *jwt.NumericDate, not time.Time. so we need to convert the unix timestamp to a *jwt.NumericDate using jwt.NewNumericDate(time.Unix(issuedAt, 0)) and jwt.NewNumericDate(time.Unix(expiresAt, 0)).
	newJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedJWT, err := newJWT.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", errors.New("couldn't sign the JWT")
	}
	return signedJWT, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error){
	claims := jwt.RegisteredClaims{}
	//what is this? jwt.ParseWithClaims takes a token string, a claims object, and a function that returns the key to use for signing the token. 
	//it parses the token string and populates the claims object with the claims from the token. if the token is valid, it returns the claims object. if the token is invalid, it returns an error.

	token, err := jwt.ParseWithClaims(
		tokenString, 
		&claims, 
		func(token *jwt.Token) (interface{}, error){
			return []byte(tokenSecret), nil
		},
	)
	//ParseWithClaims necesita saber qué key usar para verificar la firma. está guardada en el header con signingMethod, pero no es suficiente: el header no tiene la key. por eso le pasamos la función que devuelve la key.
	//toma el token y consigue el tokensecret. en nuestro caso siempre es la misma key
	if err != nil {
		return uuid.Nil, errors.New("couldn't parse the JWT")
	}
	uuidString := claims.Subject  //tenemos el uuid en formato string, lo convertimos a uuid.UUID
	parsedUUID, err := uuid.Parse(uuidString)
	if err != nil {
		return uuid.Nil, errors.New("couldn't parse the UUID from the JWT")
	}
	return parsedUUID, nil
}
