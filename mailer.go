package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

//resendMailer envía mails reales usando la API de Resend
type resendMailer struct {
	apiKey string
	from   string
	client *http.Client
}

func newResendMailer(apiKey, fromAddress string) resendMailer {
	return resendMailer{
		apiKey: apiKey,
		from:   fmt.Sprintf("Connections <%s>", fromAddress), //nombre visible + dirección
		client: &http.Client{Timeout: 10 * time.Second},     //si Resend no contesta, no colgamos la API
	}
}

func (m resendMailer) Send(to, subject, body string) error {
	payload, err := json.Marshal(map[string]any{
		"from":    m.from,
		"to":      []string{to},
		"subject": subject,
		"text":    body,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")
	//creando el request, luego lo trabajamos
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512)) //solo el comienzo de la respuesta, para el log
		return fmt.Errorf("resend: status %d: %s", resp.StatusCode, msg)
	}
	return nil
}