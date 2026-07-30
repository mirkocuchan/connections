package auth

import(
	"errors"
	"github.com/alexedwards/argon2id"
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
