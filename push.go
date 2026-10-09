package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const expoPushURL = "https://exp.host/--/api/v2/push/send" //servidor de expo a donde se pushean las notificaciones. se puede cambiar por un servidor propio si se quiere.

type expoMessage struct { //lo que se manda a expo para enviar la notificación, con la info correspondiente.
	To    string         `json:"to"`
	Title string         `json:"title"`
	Body  string         `json:"body"`
	Sound string         `json:"sound"`
	Data  map[string]any `json:"data,omitempty"`
}

type expoTicket struct { //lo que devuelve expo cuando se manda la notificación, con el status de la misma.
	Status  string `json:"status"`
	ID      string `json:"id"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type expoResponse struct { //es la envoltura de la respuesta de expo, que contiene un array de tickets con el status de cada notificación enviada.
	Data []expoTicket `json:"data"`
}

//sendPush manda una notificación a todos los dispositivos de un usuario.
//pensada para correr en una goroutine: no devuelve error, solo genera logs. 
func (s *state) sendPush(userID uuid.UUID, title, body string, data map[string]any) {
	//el context del request se cancela cuando el handler responde, entonces tenemos que crear un contexto para que no se nos cierre la ventana.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokens, err := s.db.GetPushTokensByUserID(ctx, userID)
	if err != nil {
		log.Printf("push: could not load tokens: %v", err)
		return
	}
	if len(tokens) == 0 {
		return
	}
	msgs := make([]expoMessage, 0, len(tokens))
	for _, t := range tokens {
		msgs = append(msgs, expoMessage{To: t, Title: title, Body: body, Sound: "default", Data: data})
	} //creamnos un array de mensajes para mandar a expo, uno por cada token del usuario. el usuario tiene un token por cada dispositivo que tenga registrado. si el usuario tiene 3 dispositivos, se le mandan 3 notificaciones.
	//el make medio que pide memoria para el array de mensajes, para no tener que ir haciendo append y reallocando memoria cada vez.
	payload, err := json.Marshal(msgs)
	if err != nil {
		log.Printf("push: marshal: %v", err)
		return
	} //registra los errores de marshaling del json, que no deberían pasar nunca porque los tipos son correctos.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, expoPushURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("push: request: %v", err)
		return
	} //armamos el pedido a expo, con el payload en el body. si hay error, lo registramos y salimos.
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req) //enviado
	if err != nil {
		log.Printf("push: send: %v", err)
		return
	}//registra los errores de envío del request, que pueden ser por problemas de red, etc. no son errores de la app.
	defer resp.Body.Close() //resp viene como un stream, hay que cerrarlo al final para liberar recursos. si no se cierra, se queda abierto y puede generar problemas de memoria.

	var res expoResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		log.Printf("push: decode: %v", err)
		return
	}
	//los tickets vienen en el mismo orden que los mensajes enviados, nos fijamos si cada uno de estos tickets tiene un error, y si es así, lo registramos. si el error es "DeviceNotRegistered", significa que el token ya no es válido, y lo borramos de la base de datos.
	for i, ticket := range res.Data {
		if i >= len(tokens) {
			break
		}
		if ticket.Status == "error" {
			log.Printf("push: ticket error for %s: %s", tokens[i], ticket.Message)
			if ticket.Details.Error == "DeviceNotRegistered" {
				if err := s.db.DeletePushToken(ctx, tokens[i]); err != nil {
					log.Printf("push: could not delete dead token: %v", err)
				}
			}
		}
	}
}