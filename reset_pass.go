package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
)

//mailer es "algo que sabe mandar un mail". hoy imprimimos en consola, cuando elijamos un servicio real, escribimos otra versión con el mismo método y no cambia nada más.
type Mailer interface {
	Send(to, subject, body string) error
}

//logMailer no envía nada: imprime el mail en la terminal de go run
type logMailer struct{}

func (logMailer) Send(to, subject, body string) error {
	log.Printf("MAIL a %s | %s\n%s", to, subject, body)
	return nil
}

//generateResetCode devuelve 6 dígitos al azar, con ceros a la izquierda ("004821")
func generateResetCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func hashResetCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}