package main

import (
	"net/http"
	"io"
	"github.com/mirkocuchan/connections/internal/auth"
	"github.com/mirkocuchan/connections/internal/database"
	"time"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"database/sql"
	"strings"
	"os"
	"log"
	"crypto/subtle"
	"errors"
)

type receivedUser struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DateOfBirth Date   `json:"date_of_birth"`
	AcceptTerms bool   `json:"accept_terms"`
}

func (s *state) register(w http.ResponseWriter, r *http.Request){
	defer r.Body.Close()

	userData, err := io.ReadAll(r.Body)
	if err != nil{
		RespondWithError(w, 400, "couldn't read the request body")
		return
	}
	
	var user receivedUser
	if err := json.Unmarshal(userData, &user); err != nil {
        RespondWithError(w, 400, "error unmarshalling JSON")
		return
    }
	dob := time.Time(user.DateOfBirth)
	now := time.Now().UTC()
	if dob.IsZero() || dob.After(now) || dob.Year() < 1900 {
		RespondWithError(w, 400, "Fecha de nacimiento inválida")
		return
	}
	if ageYears(dob, now) < 18 {
		RespondWithError(w, 403, "No se pudo crear la cuenta")
		return
	}

	if !user.AcceptTerms {
		RespondWithError(w, 400, "Tenés que aceptar los términos y condiciones")
		return
	}

	hashedPassword, err := auth.HashPassword(user.Password)
	if err != nil{
		RespondWithError(w, 400, "error hashing the password")
		return
	}
	//asigno un Date a un campo que espera time.Time? NO. necesito hacer una conversión: time.Time(user.DateOfBirth). 
	//válida porque Date tiene la misma estructura interna que time.Time.
	userParams := database.CreateUserParams{
		Username: user.Username,
		Email: user.Email,
		PasswordHash: hashedPassword,
		DateOfBirth: time.Time(user.DateOfBirth),
		TermsVersion: sql.NullString{
			String: currentTermsVersion,
			Valid:  true,
		},
	}
	
	createdUser, err := s.db.CreateUser(r.Context(), userParams)
	//cuando Postgres rechaza un INSERT por violar el UNIQUE de username o email, no te devuelve un error genérico de Go. 
	//te devuelve un error que tiene información específica de Postgres: un código de error estandarizado. 
	//ese código para "unique violation" es siempre 23505. el paquete lib/pq convierte esa respuesta de PostgreSQL en una estructura de Go (struct) llamada pq.Error.
	//expone esa estructura (es decir, hacer que sus campos sean públicos, con mayúscula), te permite hacer un type assertion (conversión de tipo) en Go para acceder directamente a esos campos

	if err != nil{
		//quiero comprobar si detrás de esta interfaz de error, hay un pq.Error. si lo hay quiero acceso a él con todos sus campos propios.
		//variable, ok := interfaz.(TipoConcreto). ok es bool: true si err era *pq.Error por dentro, false si no. pqErr es la variable nueva, de tipo *pq.Error
		//si ok = true, podemos acceder a pqErr.Code porque ahora Go sabe con certeza qué tipo es. ok = false, pqErr va a ser nil (no te sirve, no rompe nada)
		if pqErr, ok := err.(*pq.Error); ok {
			//pqErr es de tipo concreto pq.Err ya
			if pqErr.Code == "23505"{
				RespondWithError(w, 409, "there is another user with this username")
				return
			}else{
				RespondWithError(w, 500, "error creating a user in the database")
				return
			}
		}else{
			RespondWithError(w, 500, "error creating a user in the database")
			return
		}
	}
	type responseUser struct {
		ID uuid.UUID `json:"id"`
		Username string `json:"username"`
		Email string `json:"email"`
		DateOfBirth Date `json:"date_of_birth"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	
	RespondWithJSON(w, http.StatusCreated, responseUser{
		ID: createdUser.UserID,
		Username: createdUser.Username,
		Email: createdUser.Email,
		DateOfBirth: Date(createdUser.DateOfBirth),
		CreatedAt: createdUser.CreatedAt,
		UpdatedAt: createdUser.UpdatedAt,
	})
}

func (s *state) login(w http.ResponseWriter, r *http.Request){
	type User struct{
		Email string    `json:"email"`
		Password string `json:"password"`
		RegionCode string `json:"region_code"`
	}
	decoder := json.NewDecoder(r.Body)
	params := User{}
	
	err := decoder.Decode(&params)
	if err != nil {
		RespondWithError(w, 400, "couldn't decode the body")
		return
	}
	user, err := s.db.GetUserByEmail(r.Context(), params.Email)
	if err != nil{
		RespondWithError(w, 400, "Incorrect email or password")
		return
	}

	rightPassword, err := auth.CheckPasswordHash(params.Password, user.PasswordHash)
	if err != nil{
		RespondWithError(w, 400, "Incorrect email or password")
		return
	}
	if rightPassword == false{
		RespondWithError(w, 401, "Incorrect email or password")
		return
	}
		
	expiresIn := s.cfg.AccessTokenDuration
	newJWT, err := auth.MakeJWT(user.UserID, s.cfg.JWTSecret, expiresIn)
	if err != nil{
		RespondWithError(w, 400, "couldnt't generate the JWT")
		return
	}
	refreshTokenString, err := auth.MakeRefreshToken()
	if err != nil{
		RespondWithError(w, 400, "couldn't generate the refresh token")
		return
	}
	hashedRefreshToken := auth.HashRefreshToken(refreshTokenString)
	expiresAt := time.Now().Add(s.cfg.RefreshTokenDuration)

	newRefreshToken := database.CreateRefreshTokenParams{
		TokenHash: hashedRefreshToken,
		UserID: user.UserID,
		ExpiresAt: expiresAt,
	}

	_, err = s.db.CreateRefreshToken(r.Context(), newRefreshToken)
	if err != nil{
		RespondWithError(w, 401, "couldn't generate a refresh token")
		return
	}
	type responseUser struct {
		ID uuid.UUID `json:"id"`
		Username string `json:"username"`
		Email string `json:"email"`
		DateOfBirth Date `json:"date_of_birth"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Token string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}

	country, err := lookupCountryByIP(getClientIP(r))
	if err != nil || country == "" {
		country = normalizeRegionCode(params.RegionCode) // plan B: región del dispositivo
	}
	if country != "" { // si fallaron las dos señales, no pisamos lo que ya había
		s.db.UpdateDetectedCountry(r.Context(), database.UpdateDetectedCountryParams{
			UserID:          user.UserID,
			DetectedCountry: sql.NullString{String: country, Valid: true},
		})
	}

	RespondWithJSON(w, 200, responseUser{ID: user.UserID, Username: user.Username, Email: user.Email,
	DateOfBirth: Date(user.DateOfBirth), CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt, Token: newJWT,
	RefreshToken: refreshTokenString})
	return
}

func (s *state) refresh(w http.ResponseWriter, r *http.Request){
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
        return
	}

	hashedRefreshToken := auth.HashRefreshToken(refreshToken)
	refreshTokenData, err := s.db.GetRefreshTokenWithHash(r.Context(), hashedRefreshToken)
	if err != nil{
		RespondWithError(w, 400, "couldn't find the refresh token in the database")
		return
	}
	if refreshTokenData.ExpiresAt.Before(time.Now()){
		RespondWithError(w, 400, "the refresh token has expired")
		return
	}
	if refreshTokenData.RevokedAt.Valid{
		RespondWithError(w, 400, "the refresh token has been revoked")
		return
	}
	//si existe me fijo si está revocado o expirado. si no, genero un nuevo JWT y un nuevo refresh token, y devuelvo ambos al cliente.
	expiresIn := s.cfg.AccessTokenDuration
	newJWT, err := auth.MakeJWT(refreshTokenData.UserID, s.cfg.JWTSecret, expiresIn)
	if err != nil{
		RespondWithError(w, 400, "couldnt't generate the JWT")
		return
	}

	refreshTokenString, err := auth.MakeRefreshToken()
	if err != nil{
		RespondWithError(w, 400, "couldn't generate the refresh token")
		return
	}
	newHashedRefreshToken := auth.HashRefreshToken(refreshTokenString)
	expiresAt := time.Now().Add(s.cfg.RefreshTokenDuration)

	newRefreshToken := database.CreateRefreshTokenParams{
		TokenHash: newHashedRefreshToken,
		UserID: refreshTokenData.UserID,
		ExpiresAt: expiresAt,
	}
	_, err = s.db.CreateRefreshToken(r.Context(), newRefreshToken)
	if err != nil{
		RespondWithError(w, 401, "couldn't generate a refresh token")
		return
	}
	err = s.db.RevokeRefreshToken(r.Context(), hashedRefreshToken)
	if err != nil{
		RespondWithError(w, 400, "couldn't revoke the old refresh token")
		return
	}

	RespondWithJSON(w, 200, map[string]string{"token": newJWT, "refresh_token": refreshTokenString})
}

func (s *state) logout(w http.ResponseWriter, r *http.Request){
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
        return
	}

	hashedRefreshToken := auth.HashRefreshToken(refreshToken)
	err = s.db.RevokeRefreshToken(r.Context(), hashedRefreshToken)
	if err != nil{
		RespondWithError(w, 400, "couldn't revoke the refresh token")
		return
	}
	RespondWithJSON(w, 200, map[string]string{"message": "refresh token revoked successfully"})
}

func (s *state) getMe(w http.ResponseWriter, r *http.Request){
	//quiero obtener el userID del contexto de la request. el middleware authMiddleware lo puso ahí, así que si llegamos a este punto, el userID debería estar en el contexto.
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	type responseUser struct {
		ID uuid.UUID `json:"id"`
		Username string `json:"username"`
		Email string `json:"email"`
		DateOfBirth Date `json:"date_of_birth"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		DisplayName string    `json:"display_name"`
		Bio         string    `json:"bio"`
		City        string    `json:"city"`
		Country     string    `json:"country"`
		Hobbies     string    `json:"hobbies"`
		Languages   string    `json:"languages"`
		GlobalDiscovery bool `json:"global_discovery"`
	}
	
	RespondWithJSON(w, 200, responseUser{
		ID: user.UserID,
		Username: user.Username,
		Email: user.Email,
		DateOfBirth: Date(user.DateOfBirth),
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		DisplayName: user.DisplayName.String,
		Bio:         user.Bio.String,
		City:        user.City.String,
		Country:     user.Country.String,
		Hobbies:     user.Hobbies.String,
		Languages:   user.Languages.String,
		GlobalDiscovery: user.GlobalDiscovery,
	})
}

type createMessage struct {
    Content string `json:"content"`
}

func (s *state) createMessage(w http.ResponseWriter, r *http.Request){
	defer r.Body.Close()

	userData, err := io.ReadAll(r.Body)
	if err != nil{
		RespondWithError(w, 400, "couldn't read the request body")
		return
	}
	
	var body createMessage
	if err := json.Unmarshal(userData, &body); err != nil {
        RespondWithError(w, 400, "error unmarshalling JSON")
		return
    }
	//getting the content of the message that is being sent
	body.Content = strings.TrimSpace(body.Content)
	if body.Content == "" {
		RespondWithError(w, 400, "message can't be empty")
		return
	}
	
	//message sender
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	chatIDString := r.PathValue("chatID")
	chatID, err := uuid.Parse(chatIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid chat ID")
    	return
	}
	chat, err := s.db.GetChatByID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 404, "Chat not found")
		return
	}
	if chat.UserOneID != userID && chat.UserTwoID != userID{
		RespondWithError(w, 403, "you are not a participant of this chat")
		return
	}//me fijo si pertenece a alguno de los dos usuarios del chat. si no, devuelvo 403 forbidden.
	
	isBlocked, err := s.isBlocked(r.Context(), chat.UserOneID, chat.UserTwoID)
	if err != nil{
		RespondWithError(w, 500, "error checking block status")
		return
	}
	if isBlocked{
		RespondWithError(w, 403, "you have been blocked by this user, you can't send messages")
		return
	}

	newMessageParams := database.CreateMessageParams{
		ChatID:   chatID,
		SenderID: userID,
		Content:  body.Content,
	}
	//creamos nuevo mensaje en la base de datos. si hay error, devolvemos 500. si no, devolvemos el mensaje creado con 201.
	newMessage, err := s.db.CreateMessage(r.Context(), newMessageParams)
	if err != nil {
		RespondWithError(w, 500, "couldn't create the message in the database")
		return
	}
	recipientID := chat.UserOneID
	if recipientID == userID {
		recipientID = chat.UserTwoID
	}
	go s.notifyNewMessage(chatID, userID, recipientID, chat.UserOneID == recipientID, body.Content)
	s.emitNewMessage(chatID, recipientID, wsMessage{
		MessageID: newMessage.MessageID,
		Content:   newMessage.Content,
		CreatedAt: newMessage.CreatedAt,
	})
	type messageResponse struct {
		MessageID uuid.UUID `json:"message_id"`
		ChatID    uuid.UUID `json:"chat_id"`
		IsMine    bool      `json:"is_mine"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	RespondWithJSON(w, 201, messageResponse{
		MessageID: newMessage.MessageID,
		ChatID:    newMessage.ChatID,
		IsMine:    true,
		Content:   newMessage.Content,
		CreatedAt: newMessage.CreatedAt,
		UpdatedAt: newMessage.UpdatedAt,})
}

func (s *state) getMessages(w http.ResponseWriter, r *http.Request){
	//message sender
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	chatIDString := r.PathValue("chatID")
	chatID, err := uuid.Parse(chatIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid chat ID")
    	return
	}
	chat, err := s.db.GetChatByID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 404, "Chat not found")
		return
	}
	if chat.UserOneID != userID && chat.UserTwoID != userID{
		RespondWithError(w, 403, "you are not a participant of this chat")
		return
	}//me fijo si pertenece a alguno de los dos usuarios del chat. si no, devuelvo 403 forbidden.

	messages, err := s.db.GetMessagesByChatID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 500, "couldn't get the messages from the database")
		return
	}
	type messageResponse struct {
		MessageID uuid.UUID `json:"message_id"`
		ChatID    uuid.UUID `json:"chat_id"`
		IsMine    bool      `json:"is_mine"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		IsStoryReply   bool      `json:"is_story_reply"`
		StoryMediaURL  *string   `json:"story_media_url,omitempty"`
		StoryMediaType *string   `json:"story_media_type,omitempty"`
	}
	messagesResponse := []messageResponse{}
	for _, message := range messages {
		var storyURL, storyType *string
		if message.StoryMediaUrl.Valid {
			u := absoluteURL(r, message.StoryMediaUrl.String)
			t := message.StoryMediaType.String
			storyURL = &u
			storyType = &t
		}
		messagesResponse = append(messagesResponse, messageResponse{
			MessageID: message.MessageID,
			ChatID:    message.ChatID,
			IsMine:    message.SenderID == userID,
			Content:   message.Content,
			CreatedAt: message.CreatedAt,
			UpdatedAt: message.UpdatedAt,
			IsStoryReply:   message.IsStoryReply,
			StoryMediaURL:  storyURL,
			StoryMediaType: storyType,
		})
	}
	RespondWithJSON(w, 200, messagesResponse)
}

func (s *state) deleteChat(w http.ResponseWriter, r *http.Request){
	//the one soliciting the deletion
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	chatIDString := r.PathValue("chatID")
	chatID, err := uuid.Parse(chatIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid chat ID")
    	return
	}
	chat, err := s.db.GetChatByID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 404, "Chat not found")
		return
	}
	if chat.UserOneID != userID && chat.UserTwoID != userID{
		RespondWithError(w, 403, "you are not a participant of this chat, you can't delete it")
		return
	}//me fijo si pertenece a alguno de los dos usuarios del chat. si no, devuelvo 403 forbidden.
	
	err = s.db.DeleteChatByID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 500, "could not delete the chat")
		return
	}
	RespondWithJSON(w, 200, map[string]string{"message": "chat deleted", "chat_id": chat.ChatID.String()})
}

type updateFieldsStruct struct {
	DisplayName *string `json:"display_name"`
	Bio         *string `json:"bio"`
	City        *string `json:"city"`
	Country     *string `json:"country"`
	Hobbies     *string `json:"hobbies"`
	Languages   *string `json:"languages"`
}

func (s *state) updateFields(w http.ResponseWriter, r *http.Request){
	defer r.Body.Close()

	userData, err := io.ReadAll(r.Body)
	if err != nil{
		RespondWithError(w, 400, "couldn't read the request body")
		return
	}
	
	var body updateFieldsStruct
	if err := json.Unmarshal(userData, &body); err != nil {
        RespondWithError(w, 400, "error unmarshalling JSON")
		return
    }
	//getting the content of the update that is being done

	//the one that is going to edit the fields
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	updateUserParams := database.UpdateUserParams{
		DisplayName: nullString(body.DisplayName),
		Bio:         nullString(body.Bio),
		City:        nullString(body.City),
		Country:     nullString(body.Country),
		Hobbies:     nullString(body.Hobbies),
		Languages:   nullString(body.Languages),
		UserID:      userID,
	}
	updatedUser, err := s.db.UpdateUser(r.Context(), updateUserParams)
	if err != nil{
		RespondWithError(w, 500, "Internal Server Error")
		return
	}
	type responseUser struct {
		ID uuid.UUID `json:"id"`
		Username string `json:"username"`
		Email string `json:"email"`
		DateOfBirth Date `json:"date_of_birth"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		DisplayName string `json:"display_name"`
		Bio string `json:"bio"`
		City string `json:"city"`
		Country string `json:"country"`
		Hobbies string `json:"hobbies"`
		Languages string `json:"languages"`
	}
	RespondWithJSON(w, 200, responseUser{
    ID:          updatedUser.UserID,
    Username:    updatedUser.Username,
    Email:       updatedUser.Email,
    DateOfBirth: Date(updatedUser.DateOfBirth),
    CreatedAt:   updatedUser.CreatedAt,
    UpdatedAt:   updatedUser.UpdatedAt,
    DisplayName: updatedUser.DisplayName.String,
    Bio:         updatedUser.Bio.String,
    City:        updatedUser.City.String,
    Country:     updatedUser.Country.String,
    Hobbies:     updatedUser.Hobbies.String,
    Languages:   updatedUser.Languages.String,
	})
}

func (s *state) getUserCard(w http.ResponseWriter, r *http.Request){
	//the one that is requesting the card
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	chatIDString := r.PathValue("chatID")
	chatID, err := uuid.Parse(chatIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid chat ID")
    	return
	}
	
	chat, err := s.db.GetChatByID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 404, "chat doesn't exist in the database")
    	return
	}

	var subjectID uuid.UUID
	if chat.UserOneID == userID {
		subjectID = chat.UserTwoID
	} else {
		subjectID = chat.UserOneID
	}

	if chat.UserOneID != userID && chat.UserTwoID != userID{
		RespondWithError(w, 403, "you are not a participant of this chat, you can't delete it")
		return
	}//me fijo si pertenece a alguno de los dos usuarios del chat. si no, devuelvo 403 forbidden.
	isBlocked, err := s.isBlocked(r.Context(), userID, subjectID)
	if err != nil{
		RespondWithError(w, 500, "error checking block status")
		return
	}
	if isBlocked{
		RespondWithError(w, 403, "you can't access this profile")
		return
	}
	//cardParams
	cardParams := database.GetCardWithChatCreatorAndSubjectParams{
		ChatID: chatID,
		CreatorID: userID,
		SubjectID: subjectID,
	}
	type responseCard struct {
		CardID             uuid.UUID `json:"card_id"`
		ChatID             uuid.UUID `json:"chat_id"`
		IsInitiator        bool       `json:"is_initiator"`
		ChatName           string     `json:"chat_name"`
		SubjectID          *uuid.UUID `json:"subject_id,omitempty"` //solo para el iniciador
		Nickname           sql.NullString `json:"nickname"`
		NotesOnSubject     sql.NullString `json:"notes_on_subject"`
		DisplayNameVisible sql.NullBool `json:"display_name"`
		DateOfBirthVisible sql.NullBool `json:"date_of_birth"`
		CityVisible        sql.NullBool `json:"city"`
		CountryVisible     sql.NullBool `json:"country"`
		PhotosVisible      sql.NullBool `json:"photos"`
		BioVisible         sql.NullBool `json:"bio"`
		HobbiesVisible     sql.NullBool `json:"hobbies"`
		LanguagesVisible   sql.NullBool `json:"languages"`
		DisplayName       string `json:"display_name_value"`
		DateOfBirth       string `json:"date_of_birth_value"`
		City              string `json:"city_value"`
		Country           string `json:"country_value"`
		Bio               string `json:"bio_value"`
		Hobbies           string `json:"hobbies_value"`
		Languages         string `json:"languages_value"`
	}
	isInitiator := chat.UserOneID == userID
	var subjectForClient *uuid.UUID
	if isInitiator {
		subjectForClient = &subjectID
	}

	card, err := s.db.GetCardWithChatCreatorAndSubject(r.Context(), cardParams)
	if err == sql.ErrNoRows{
		//si no hay card, la creo. si hay card, devuelvo la card existente.
		newCardParams := database.CreateUserCardParams{
			ChatID: chatID,
			CreatorID: userID,
			SubjectID: subjectID,
		}

		newCard, err := s.db.CreateUserCard(r.Context(), newCardParams)
		if err != nil{
			RespondWithError(w, 500, "couldn't create the card in the database")
			return
		}
		chatName := s.displayNameFor(r.Context(), chatID, userID, subjectID, isInitiator)
		RespondWithJSON(w, 200, responseCard{CardID: newCard.CardID, ChatID: chat.ChatID, 
		IsInitiator: isInitiator, ChatName: chatName, SubjectID: subjectForClient, Nickname: newCard.Nickname,
		NotesOnSubject: newCard.NotesOnSubject, DisplayNameVisible: newCard.DisplayNameVisible, DateOfBirthVisible: newCard.DateOfBirthVisible, 
		CityVisible: newCard.CityVisible, CountryVisible: newCard.CountryVisible, PhotosVisible: newCard.PhotosVisible, BioVisible: newCard.BioVisible, HobbiesVisible: newCard.HobbiesVisible, 
		LanguagesVisible: newCard.LanguagesVisible, DisplayName: "", DateOfBirth: "", City: "", Country: "", Bio: "", Hobbies: "", Languages: ""})
		return
	}
	
	if err != nil{
		RespondWithError(w, 500, "couldn't find the card in the database")
		return
	}
	getCardWithSubjectData, err := s.db.GetCardWithSubjectData(r.Context(), card.CardID)
	if err != nil{
		RespondWithError(w, 500, "couldn't get the card data")
		return
	}
	chatName := s.displayNameFor(r.Context(), chatID, userID, subjectID, isInitiator)
	RespondWithJSON(w, 200, responseCard{CardID: card.CardID, ChatID: chat.ChatID, IsInitiator: isInitiator, ChatName: chatName, SubjectID: subjectForClient, Nickname: card.Nickname,
	NotesOnSubject: card.NotesOnSubject, DisplayNameVisible: getCardWithSubjectData.DisplayNameVisible, DateOfBirthVisible: getCardWithSubjectData.DateOfBirthVisible, 
	CityVisible: getCardWithSubjectData.CityVisible, CountryVisible: getCardWithSubjectData.CountryVisible, PhotosVisible: getCardWithSubjectData.PhotosVisible, BioVisible: getCardWithSubjectData.BioVisible, HobbiesVisible: getCardWithSubjectData.HobbiesVisible, 
	LanguagesVisible: getCardWithSubjectData.LanguagesVisible, DisplayName: revealOrHidden(getCardWithSubjectData.DisplayNameVisible, getCardWithSubjectData.DisplayName),
	DateOfBirth: revealOrHiddenDOB(getCardWithSubjectData.DateOfBirthVisible, getCardWithSubjectData.DateOfBirth),
	City: revealOrHidden(getCardWithSubjectData.CityVisible, getCardWithSubjectData.City), Country: revealOrHidden(getCardWithSubjectData.CountryVisible, getCardWithSubjectData.Country),
	Bio: revealOrHidden(getCardWithSubjectData.BioVisible, getCardWithSubjectData.Bio), Hobbies: revealOrHidden(getCardWithSubjectData.HobbiesVisible, getCardWithSubjectData.Hobbies),
	Languages: revealOrHidden(getCardWithSubjectData.LanguagesVisible, getCardWithSubjectData.Languages)})
}

type createNickname struct {
    Nickname string `json:"nickname"`
}

func (s *state) updateNickname(w http.ResponseWriter, r *http.Request){
	defer r.Body.Close()

	nicknameData, err := io.ReadAll(r.Body)
	if err != nil{
		RespondWithError(w, 400, "couldn't read the request body")
		return
	}
	
	var body createNickname
	if err := json.Unmarshal(nicknameData, &body); err != nil {
        RespondWithError(w, 400, "error unmarshalling JSON")
		return
    }
	//getting the content of the nickname that is being updated

	//the one that is requesting the card
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	chatIDString := r.PathValue("chatID")
	chatID, err := uuid.Parse(chatIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid chat ID")
    	return
	}
	
	chat, err := s.db.GetChatByID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 404, "chat doesn't exist in the database")
    	return
	}

	var subjectID uuid.UUID
	if chat.UserOneID == userID {
		subjectID = chat.UserTwoID
	} else {
		subjectID = chat.UserOneID
	}

	if chat.UserOneID != userID && chat.UserTwoID != userID{
		RespondWithError(w, 403, "you are not a participant of this chat, you can't delete it")
		return
	}//me fijo si pertenece a alguno de los dos usuarios del chat. si no, devuelvo 403 forbidden.

	//cardParams
	cardParams := database.GetCardWithChatCreatorAndSubjectParams{
		ChatID: chatID,
		CreatorID: userID,
		SubjectID: subjectID,
	}
	
	card, err := s.db.GetCardWithChatCreatorAndSubject(r.Context(), cardParams)
	if err == sql.ErrNoRows{
		RespondWithError(w, 500, "card does not exist or couldn't get it")
		return
	}
	
	updateNicknameParams := database.UpdateNicknameParams{
		Nickname: sql.NullString{
		String: body.Nickname,
		Valid:  true,},
		CardID: card.CardID,
	}
	updatedCard, err := s.db.UpdateNickname(r.Context(), updateNicknameParams)
	if err != nil{
		RespondWithError(w, 500, "couldn't update the nickname")
		return
	}
	type responseCard struct {
		CardID             uuid.UUID `json:"card_id"`
		ChatID             uuid.UUID `json:"chat_id"`
		Nickname           sql.NullString `json:"nickname"`
		NotesOnSubject     sql.NullString `json:"notes_on_subject"`
		DisplayNameVisible sql.NullBool `json:"display_name"`
		DateOfBirthVisible sql.NullBool `json:"date_of_birth"`
		CityVisible        sql.NullBool `json:"city"`
		CountryVisible     sql.NullBool `json:"country"`
		PhotosVisible      sql.NullBool `json:"photos"`
		BioVisible         sql.NullBool `json:"bio"`
		HobbiesVisible     sql.NullBool `json:"hobbies"`
		LanguagesVisible   sql.NullBool `json:"languages"`
		CreatedAt          time.Time `json:"created_at"`
		UpdatedAt          time.Time `json:"updated_at"`
	}
	RespondWithJSON(w, 200, responseCard{CardID: updatedCard.CardID, ChatID: updatedCard.ChatID, Nickname: updatedCard.Nickname, NotesOnSubject: updatedCard.NotesOnSubject, DisplayNameVisible: updatedCard.DisplayNameVisible, DateOfBirthVisible: updatedCard.DateOfBirthVisible,
		CityVisible: updatedCard.CityVisible, CountryVisible: updatedCard.CountryVisible, PhotosVisible: updatedCard.PhotosVisible, BioVisible: updatedCard.BioVisible, HobbiesVisible: updatedCard.HobbiesVisible,
		LanguagesVisible: updatedCard.LanguagesVisible, CreatedAt: updatedCard.CreatedAt, UpdatedAt: updatedCard.UpdatedAt})
}

type createNotes struct {
    Notes string `json:"notes_on_subject"`
}

func (s *state) updateNotesOnSubject(w http.ResponseWriter, r *http.Request){
	defer r.Body.Close()

	messageData, err := io.ReadAll(r.Body)
	if err != nil{
		RespondWithError(w, 400, "couldn't read the request body")
		return
	}
	
	var body createNotes
	if err := json.Unmarshal(messageData, &body); err != nil {
        RespondWithError(w, 400, "error unmarshalling JSON")
		return
    }
	//getting the content of the notes that is being updated

	//the one that is requesting the card
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	chatIDString := r.PathValue("chatID")
	chatID, err := uuid.Parse(chatIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid chat ID")
    	return
	}
	
	chat, err := s.db.GetChatByID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 404, "chat doesn't exist in the database")
    	return
	}

	var subjectID uuid.UUID
	if chat.UserOneID == userID {
		subjectID = chat.UserTwoID
	} else {
		subjectID = chat.UserOneID
	}

	if chat.UserOneID != userID && chat.UserTwoID != userID{
		RespondWithError(w, 403, "you are not a participant of this chat, you can't delete it")
		return
	}//me fijo si pertenece a alguno de los dos usuarios del chat. si no, devuelvo 403 forbidden.

	//cardParams
	cardParams := database.GetCardWithChatCreatorAndSubjectParams{
		ChatID: chatID,
		CreatorID: userID,
		SubjectID: subjectID,
	}
	
	card, err := s.db.GetCardWithChatCreatorAndSubject(r.Context(), cardParams)
	if err == sql.ErrNoRows{
		RespondWithError(w, 500, "card does not exist or couldn't get it")
		return
	}
	updateNotesOnSubjectParams := database.UpdateNotesOnSubjectParams{
		NotesOnSubject: sql.NullString{
		String: body.Notes,
		Valid:  true,},
		CardID: card.CardID,
	}
	updatedCard, err := s.db.UpdateNotesOnSubject(r.Context(), updateNotesOnSubjectParams)
	if err != nil{
		RespondWithError(w, 500, "couldn't update the notes")
		return
	}
	type responseCard struct {
		CardID             uuid.UUID `json:"card_id"`
		ChatID             uuid.UUID `json:"chat_id"`
		Nickname           sql.NullString `json:"nickname"`
		NotesOnSubject     sql.NullString `json:"notes_on_subject"`
		DisplayNameVisible sql.NullBool `json:"display_name"`
		DateOfBirthVisible sql.NullBool `json:"date_of_birth"`
		CityVisible        sql.NullBool `json:"city"`
		CountryVisible     sql.NullBool `json:"country"`
		PhotosVisible      sql.NullBool `json:"photos"`
		BioVisible         sql.NullBool `json:"bio"`
		HobbiesVisible     sql.NullBool `json:"hobbies"`
		LanguagesVisible   sql.NullBool `json:"languages"`
		CreatedAt          time.Time `json:"created_at"`
		UpdatedAt          time.Time `json:"updated_at"`
	}
	RespondWithJSON(w, 200, responseCard{CardID: updatedCard.CardID, ChatID: updatedCard.ChatID, Nickname: updatedCard.Nickname, NotesOnSubject: updatedCard.NotesOnSubject, 
		DisplayNameVisible: updatedCard.DisplayNameVisible, DateOfBirthVisible: updatedCard.DateOfBirthVisible, CityVisible: updatedCard.CityVisible, 
		CountryVisible: updatedCard.CountryVisible, PhotosVisible: updatedCard.PhotosVisible, BioVisible: updatedCard.BioVisible, HobbiesVisible: updatedCard.HobbiesVisible,
		LanguagesVisible: updatedCard.LanguagesVisible, CreatedAt: updatedCard.CreatedAt, UpdatedAt: updatedCard.UpdatedAt})
}

func (s *state) revealAField(w http.ResponseWriter, r *http.Request){
	//the one that is revealing the field
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	fieldToReveal := r.PathValue("field")
	chatIDString := r.PathValue("chatID")
	chatID, err := uuid.Parse(chatIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid chat ID")
    	return
	}
	
	chat, err := s.db.GetChatByID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 404, "chat doesn't exist in the database")
    	return
	}

	var subjectID uuid.UUID
	if chat.UserOneID == userID {
		subjectID = chat.UserTwoID
	} else {
		subjectID = chat.UserOneID
	}

	if chat.UserOneID != userID && chat.UserTwoID != userID{
		RespondWithError(w, 403, "you are not a participant of this chat, you can't delete it")
		return
	}//me fijo si pertenece a alguno de los dos usuarios del chat. si no, devuelvo 403 forbidden.

	//cardParams
	cardParams := database.GetCardWithChatCreatorAndSubjectParams{
		ChatID: chatID,
		CreatorID: subjectID,
		SubjectID: userID,
	}
	
	card, err := s.db.GetCardWithChatCreatorAndSubject(r.Context(), cardParams)
	if err == sql.ErrNoRows {
		newCardParams := database.CreateUserCardParams{
			ChatID:    chatID,
			CreatorID: subjectID,
			SubjectID: userID,
		}

		card, err = s.db.CreateUserCard(r.Context(), newCardParams)
		if err != nil {
			RespondWithError(w, 500, "couldn't create the card in the database")
			return
		}
	} else if err != nil {
		RespondWithError(w, 500, "couldn't find the card in the database")
		return
	}
	switch fieldToReveal {
		case "city":
			err = s.db.RevealCityField(r.Context(), card.CardID) 
			if err != nil{
				RespondWithError(w, 500, "couldn't reveal that field")
				return
			}
		case "country":
			err = s.db.RevealCountryField(r.Context(), card.CardID)
			if err != nil{
				RespondWithError(w, 500, "couldn't reveal that field")
				return
			}
		case "bio":
			err = s.db.RevealBioField(r.Context(), card.CardID)
			if err != nil{
				RespondWithError(w, 500, "couldn't reveal that field")
				return
			}
		case "date_of_birth":
			err = s.db.RevealBirthField(r.Context(), card.CardID)
			if err != nil{
				RespondWithError(w, 500, "couldn't reveal that field")
				return
			}
		case "name":
			err = s.db.RevealNameField(r.Context(), card.CardID)
			if err != nil{
				RespondWithError(w, 500, "couldn't reveal that field")
				return
			}
		case "hobbies":
			err = s.db.RevealHobbiesField(r.Context(), card.CardID)
			if err != nil{
				RespondWithError(w, 500, "couldn't reveal that field")
				return
			}
		case "languages":
			err = s.db.RevealLanguagesField(r.Context(), card.CardID)
			if err != nil{
				RespondWithError(w, 500, "couldn't reveal that field")
				return
			}
		case "photos":
			err = s.db.RevealPhotosField(r.Context(), card.CardID)
			if err != nil{
				RespondWithError(w, 500, "couldn't reveal that field")
				return
			}
		default:
			RespondWithError(w, 400, "invalid field")
			return
	}
	RespondWithJSON(w, 200, map[string]string{"message": "field revealed", "card_id": card.CardID.String()})
}

func (s *state) revealAllFields(w http.ResponseWriter, r *http.Request){
	//the one that is revealing the fields
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	chatIDString := r.PathValue("chatID")
	chatID, err := uuid.Parse(chatIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid chat ID")
    	return
	}
	
	chat, err := s.db.GetChatByID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 404, "chat doesn't exist in the database")
    	return
	}

	var subjectID uuid.UUID
	if chat.UserOneID == userID {
		subjectID = chat.UserTwoID
	} else {
		subjectID = chat.UserOneID
	}

	if chat.UserOneID != userID && chat.UserTwoID != userID{
		RespondWithError(w, 403, "you are not a participant of this chat, you can't delete it")
		return
	}//me fijo si pertenece a alguno de los dos usuarios del chat. si no, devuelvo 403 forbidden.

	//cardParams
	cardParams := database.GetCardWithChatCreatorAndSubjectParams{
		ChatID: chatID,
		CreatorID: subjectID,
		SubjectID: userID,
	}
	
	card, err := s.db.GetCardWithChatCreatorAndSubject(r.Context(), cardParams)
	if err == sql.ErrNoRows {
		newCardParams := database.CreateUserCardParams{
			ChatID:    chatID,
			CreatorID: subjectID,
			SubjectID: userID,
		}

		card, err = s.db.CreateUserCard(r.Context(), newCardParams)
		if err != nil {
			RespondWithError(w, 500, "couldn't create the card in the database")
			return
		}
	} else if err != nil {
		RespondWithError(w, 500, "couldn't find the card in the database")
		return
	}
	err = s.db.RevealFields(r.Context(), card.CardID)
	if err != nil{
		RespondWithError(w, 500, "couldn't reveal the fields of this card")
		return
	}

	RespondWithJSON(w, 200, map[string]string{"message": "fields revealed", "card_id": card.CardID.String()})
}

func (s *state) resetCard(w http.ResponseWriter, r *http.Request){
	//the one that is reseting the card
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	chatIDString := r.PathValue("chatID")
	chatID, err := uuid.Parse(chatIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid chat ID")
    	return
	}
	
	chat, err := s.db.GetChatByID(r.Context(), chatID)
	if err != nil{
		RespondWithError(w, 404, "chat doesn't exist in the database")
    	return
	}

	var subjectID uuid.UUID
	if chat.UserOneID == userID {
		subjectID = chat.UserTwoID
	} else {
		subjectID = chat.UserOneID
	}

	if chat.UserOneID != userID && chat.UserTwoID != userID{
		RespondWithError(w, 403, "you are not a participant of this chat, you can't delete it")
		return
	}//me fijo si pertenece a alguno de los dos usuarios del chat. si no, devuelvo 403 forbidden.

	//cardParams
	cardParams := database.GetCardWithChatCreatorAndSubjectParams{
		ChatID: chatID,
		CreatorID: userID,
		SubjectID: subjectID,
	}
	
	card, err := s.db.GetCardWithChatCreatorAndSubject(r.Context(), cardParams)
	if err == sql.ErrNoRows{
		RespondWithError(w, 500, "card does not exist or couldn't get it")
		return
	}
	err = s.db.ResetCard(r.Context(), card.CardID)
	if err != nil{
		RespondWithError(w, 500, "couldn't reset the card")
		return
	}
	RespondWithJSON(w, 200, map[string]string{"message": "card reset", "card_id": card.CardID.String()})
}

func (s *state) getChats(w http.ResponseWriter, r *http.Request){
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	chats, err := s.db.GetChatsByUserID(r.Context(), userID)
	if err != nil{
		RespondWithError(w, 500, "couldn't get chats")
    	return
	}

	type chatResponse struct {
		ChatID       uuid.UUID `json:"chat_id"`
		IsInitiator  bool      `json:"is_initiator"`
		Nickname     string    `json:"nickname"`
		LastMessage  string    `json:"last_message"`
		PhotoURL     string    `json:"photo_url"`
		Blocked		 bool 	   `json:"blocked"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
	}
	chatsResponse := []chatResponse{}
	for _, chat := range chats{
		var otherUserID uuid.UUID
		if chat.UserOneID == userID {
			otherUserID = chat.UserTwoID
		} else {
			otherUserID = chat.UserOneID
		}

		cardParams := database.GetCardWithChatCreatorAndSubjectParams{
			ChatID:    chat.ChatID,
			CreatorID: userID,
			SubjectID: otherUserID,
		}
		card, err := s.db.GetCardWithChatCreatorAndSubject(r.Context(), cardParams)
		cardOK := err == nil

		displayName := s.displayNameFor(r.Context(), chat.ChatID, userID, otherUserID, chat.UserOneID == userID)

		lastMessage, err := s.db.GetLastMessageByChatID(r.Context(), chat.ChatID)
		lastMessageContent := ""
		if err == nil {
			lastMessageContent = lastMessage.Content
		}

		// el iniciador ve la foto de quien eligió, el receptor solo si el dueño la reveló
		canSeePhoto := chat.UserOneID == userID
		if !canSeePhoto && cardOK {
			data, err := s.db.GetCardWithSubjectData(r.Context(), card.CardID)
			if err == nil && data.PhotosVisible.Valid && data.PhotosVisible.Bool {
				canSeePhoto = true
			}
		}

		photoURL := ""
		if canSeePhoto {
			photo, err := s.db.GetPrimaryUserPhoto(r.Context(), otherUserID)
			if err == nil {
				photoURL = absoluteURL(r, photo.PhotoUrl)
			}
		}

		blocked, err := s.hasBlocked(r.Context(), userID, otherUserID)
		if err != nil {
			RespondWithError(w, 500, "error checking block status")
			return
		}
		chatsResponse = append(chatsResponse, chatResponse{
			ChatID:      chat.ChatID,
			IsInitiator: chat.UserOneID == userID,
			Nickname:    displayName,
			LastMessage: lastMessageContent,
			PhotoURL:    photoURL,
			Blocked:  	 blocked,
			CreatedAt:   chat.CreatedAt,
			UpdatedAt:   chat.UpdatedAt,
		})
	}

	RespondWithJSON(w, 200, chatsResponse)
}

type createPhotoStruct struct {
	PhotoURL string `json:"photo_url"`
	Position int `json:"position"`
}
func (s *state) uploadPhoto(w http.ResponseWriter, r *http.Request){
	defer r.Body.Close()

	imageData, err := io.ReadAll(r.Body)
	if err != nil{
		RespondWithError(w, 400, "couldn't read the request body")
		return
	}
	
	var body createPhotoStruct
	if err := json.Unmarshal(imageData, &body); err != nil {
        RespondWithError(w, 400, "error unmarshalling JSON")
		return
    }
	//getting the photo that is being created
	
	//userID is the one that is uploading the photo
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	photoParams := database.CreateUserPhotoParams{
		UserID:   userID,
		PhotoUrl: body.PhotoURL,
		Position: int32(body.Position),
	}

	userPhoto, err := s.db.CreateUserPhoto(r.Context(), photoParams)
	if err != nil{
		RespondWithError(w, 500, "couldn't create the photo in the database")
		return
	}
	type responsePhoto struct {
		PhotoID uuid.UUID `json:"photo_id"`
		UserID  uuid.UUID `json:"user_id"`
		PhotoUrl string   `json:"photo_url"`
		Position int32    `json:"position"`
		CreatedAt time.Time `json:"created_at"`
	}
	RespondWithJSON(w, 201, responsePhoto{PhotoID: userPhoto.PhotoID, UserID: userPhoto.UserID, PhotoUrl: absoluteURL(r, userPhoto.PhotoUrl), Position: userPhoto.Position, CreatedAt: userPhoto.CreatedAt})
}

func (s *state) getPhotos(w http.ResponseWriter, r *http.Request){
	//userID's photos
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	photos, err := s.db.GetUserPhotos(r.Context(), userID)
	if err != nil{
		RespondWithError(w, 404, "photos don't exist in the database")
		return
	}
	type photoResponse struct {
		PhotoID   uuid.UUID `json:"photo_id"`
		UserID    uuid.UUID `json:"user_id"`
		PhotoURL  string    `json:"photo_url"`
		Position  int32     `json:"position"`
		CreatedAt time.Time `json:"created_at"`
	}
	photosResponse := []photoResponse{}
	for _, photo := range photos {
		photosResponse = append(photosResponse, photoResponse{
			PhotoID:   photo.PhotoID,
			UserID:    photo.UserID,
			PhotoURL: absoluteURL(r, photo.PhotoUrl),
			Position:  photo.Position,
			CreatedAt: photo.CreatedAt,
		})
	}
	RespondWithJSON(w, 200, photosResponse)
}

func (s *state) deletePhoto(w http.ResponseWriter, r *http.Request){
	//userID's photos
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	photoIDString := r.PathValue("photoID")
	photoID, err := uuid.Parse(photoIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid photo ID")
    	return
	}

	getUserPhotoByIDParams := database.GetUserPhotoByIDParams{
		PhotoID: photoID,
		UserID:  userID,
	}

	userPhoto, err := s.db.GetUserPhotoByID(r.Context(), getUserPhotoByIDParams)
	if err != nil {
		RespondWithError(w, 404, "Photo not found")
		return
	}
	count, err := s.db.CountUserPhotos(r.Context(), userID)
	if err != nil {
		RespondWithError(w, 500, "couldn't check photos")
		return
	}
	if count <= 1 {
		RespondWithError(w, 400, "you must keep at least one photo")
		return
	}
	deleteParams := database.DeleteUserPhotoParams{
		PhotoID: userPhoto.PhotoID,
		UserID:  userID,
	}
	err = s.db.DeleteUserPhoto(r.Context(), deleteParams)
	if err != nil{
		RespondWithError(w, 500, "could not delete the photo")
		return
	}
	RespondWithJSON(w, 200, map[string]string{"message": "photo deleted successfully"})
}

func (s *state) discoverUsers(w http.ResponseWriter, r *http.Request){
	//userID's discovery
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	currentUser, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil{
		RespondWithError(w, 500, "couldn't get your profile")
		return
	}
	discoverableUsers, err := s.db.GetDiscoverableUsers(r.Context(), database.GetDiscoverableUsersParams{
		UserID:          userID,
		GlobalDiscovery: currentUser.GlobalDiscovery,
	})
	if err != nil{
		RespondWithError(w, 500, "could not discover users")
		return
	}
	
	type discoverUserResponse struct {
		UserID      uuid.UUID `json:"user_id"`
		DisplayName string    `json:"display_name"`
		PhotoURL    string    `json:"photo_url"`
	}
	usersResponse := []discoverUserResponse{}
	for _, user := range discoverableUsers {
		isBlocked, err := s.isBlocked(r.Context(), userID, user.UserID)
		if err != nil{
			RespondWithError(w, 500, "error checking block status")
			return
		}
		if isBlocked{
			continue
		}
		primaryPhoto, err := s.db.GetPrimaryUserPhoto(r.Context(), user.UserID)
		if err != nil {
			continue
		}

		usersResponse = append(usersResponse, discoverUserResponse{
			UserID:      user.UserID,
			DisplayName: user.Username,
			PhotoURL: absoluteURL(r, primaryPhoto.PhotoUrl),
		})
	}
	RespondWithJSON(w, 200, usersResponse)
}
type createStoryStruct struct {
	MediaURL  string `json:"media_url"`
	MediaType string `json:"media_type"`
}

func (s *state) createStory(w http.ResponseWriter, r *http.Request){
	defer r.Body.Close()
	//userID's story going to be created
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	storyData, err := io.ReadAll(r.Body)
	if err != nil{
		RespondWithError(w, 400, "couldn't read the request body")
		return
	}
	
	var body createStoryStruct
	if err := json.Unmarshal(storyData, &body); err != nil {
        RespondWithError(w, 400, "error unmarshalling JSON")
		return
    }
	//getting the story details that is being created

	expiresAt := time.Now().Add(24 * time.Hour)
	createStoryParams := database.CreateStoryParams{
		UserID:    userID,
		MediaUrl:  body.MediaURL,
		MediaType: body.MediaType,
		ExpiresAt: expiresAt,
	}
	story, err := s.db.CreateStory(r.Context(), createStoryParams)
	if err != nil{
		RespondWithError(w, 500, "couldn't create the story in the database")
		return
	}

	RespondWithJSON(w, 201, map[string]interface{}{"message": "story created", "story": story})
}

func (s *state) getActiveStories(w http.ResponseWriter, r *http.Request){
	//active stories for the logged-in user
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	stories, err := s.db.GetStoryViewsByUserID(r.Context(), userID)
	if err != nil {
		RespondWithError(w, 500, "couldn't get the active stories from the database")
		return
	}
	type storyResponse struct {
		StoryID   uuid.UUID `json:"story_id"`
		UserID    uuid.UUID `json:"user_id"`
		Username  string     `json:"username"`
		MediaUrl  string    `json:"media_url"`
		MediaType string    `json:"media_type"`
		CreatedAt time.Time `json:"created_at"`
		ExpiresAt time.Time `json:"expires_at"`
		ViewedAt  *time.Time `json:"viewed_at,omitempty"`
	}
	storiesResponse := []storyResponse{}
	for _, story := range stories {
		var viewedAt *time.Time
		if story.ViewedAt.Valid {
			viewedAt = &story.ViewedAt.Time
		}
		// no mostrar historias de bloqueados
		if story.UserID != userID {
			blocked, err := s.isBlocked(r.Context(), userID, story.UserID)
			if err != nil || blocked {
				continue
			}
		}
		author, err := s.db.GetUserByID(r.Context(), story.UserID)
		username := "usuario"
		if err == nil {
			username = author.Username
		}

		storiesResponse = append(storiesResponse, storyResponse{
			StoryID:   story.StoryID,
			UserID:    story.UserID,
			Username:  username,
			MediaUrl: absoluteURL(r, story.MediaUrl),
			MediaType: story.MediaType,
			CreatedAt: story.CreatedAt,
			ExpiresAt: story.ExpiresAt,
			ViewedAt:  viewedAt,
		})
	}
	RespondWithJSON(w, 200, storiesResponse)
}

func (s *state) viewStory(w http.ResponseWriter, r *http.Request){
	//userID is the one that is viewing the story
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	storyIDString := r.PathValue("storyID")
	storyID, err := uuid.Parse(storyIDString)
	if err != nil {
		RespondWithError(w, 404, "Invalid story ID")
		return
	}
	// busco la historia para saber de quién es
	story, err := s.db.GetStoryByID(r.Context(), storyID)
	if err != nil {
		RespondWithError(w, 404, "Story not found")
		return
	}

	// si hay bloqueo, no puedo ver ni registrar la vista
	if story.UserID != userID {
		blocked, err := s.isBlocked(r.Context(), userID, story.UserID)
		if err != nil {
			RespondWithError(w, 500, "error checking permissions")
			return
		}
		if blocked {
			RespondWithError(w, 404, "Story not found")
			return
		}
	}
	registerStoryViewParams := database.RegisterStoryViewParams{
		StoryID:  storyID,
		ViewerID: userID,
	}
	err = s.db.RegisterStoryView(r.Context(), registerStoryViewParams)
	if err != nil {
		RespondWithError(w, 500, "couldn't register story view")
		return
	}
	RespondWithJSON(w, 200, map[string]interface{}{"status": "success"})
}

func (s *state) deleteStory(w http.ResponseWriter, r *http.Request){
	//userID is the one that is deleting the story
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	storyIDString := r.PathValue("storyID")
	storyID, err := uuid.Parse(storyIDString)
	if err != nil {
    	RespondWithError(w, 404, "Invalid story ID")
    	return
	}
	
	story, err := s.db.GetStoryByID(r.Context(), storyID)
	if err != nil {
		RespondWithError(w, 404, "Story not found")
		return
	}
	if story.UserID != userID {
		RespondWithError(w, 403, "Forbidden")
		return
	}
	deleteStoryByIDParams := database.DeleteStoryByIDParams{
		StoryID: storyID,
		UserID:  userID,
	}

	err = s.db.DeleteStoryByID(r.Context(), deleteStoryByIDParams)
	if err != nil {
		RespondWithError(w, 500, "couldn't delete story")
		return
	}
	removeUploadedFile(userID, story.MediaUrl)
	RespondWithJSON(w, 200, map[string]interface{}{"status": "success"})
}

func (s *state) blockUser(w http.ResponseWriter, r *http.Request){
	//userID is the one that is blocking another user
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	blockedUserIDString := r.PathValue("userID")
	blockedUserID, err := uuid.Parse(blockedUserIDString)
	if err != nil {
		RespondWithError(w, 404, "Invalid user ID")
		return
	}
	if userID == blockedUserID { 
		RespondWithError(w, 400, "you can't block yourself")
		return
	}
	createBlockParams := database.CreateBlockParams{
		BlockerID: userID,
		BlockedID: blockedUserID,
	}
	_, err = s.db.CreateBlock(r.Context(), createBlockParams)
	if err != nil {
		RespondWithError(w, 500, "couldn't block user")
		return
	}
	RespondWithJSON(w, 200, map[string]interface{}{"status": "success"})
}

func (s *state) unblockUser(w http.ResponseWriter, r *http.Request){
	//userID is the one that is unblocking another user
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	unblockedUserIDString := r.PathValue("userID")
	unblockedUserID, err := uuid.Parse(unblockedUserIDString)
	if err != nil {
		RespondWithError(w, 404, "Invalid user ID")
		return
	}
	deleteBlockByBlockerAndBlockedIDParams := database.DeleteBlockByBlockerAndBlockedIDParams{
		BlockerID: userID,
		BlockedID: unblockedUserID,
	}
	err = s.db.DeleteBlockByBlockerAndBlockedID(r.Context(), deleteBlockByBlockerAndBlockedIDParams)
	if err != nil {
		RespondWithError(w, 500, "couldn't unblock user")
		return
	}
	RespondWithJSON(w, 200, map[string]interface{}{"status": "success"})
}

func (s *state) getBlockedUsers(w http.ResponseWriter, r *http.Request){
	//userID is the one that is asking for their blocked list
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	blockedUsers, err := s.db.GetBlocksByBlockerID(r.Context(), userID)
	if err != nil {
		RespondWithError(w, 500, "couldn't get blocked users")
		return
	}
	type blockResponse struct {
		BlockedID *uuid.UUID `json:"blocked_id,omitempty"`
		ChatID    *uuid.UUID `json:"chat_id,omitempty"`
		Username  string     `json:"username"`
		CreatedAt time.Time  `json:"created_at"`
	}
	blockedResponse := []blockResponse{}
	for _, b := range blockedUsers {
		username := "usuario"
		if u, err := s.db.GetUserByID(r.Context(), b.BlockedID); err == nil {
			username = u.Username
		}
		id := b.BlockedID
		blockedID := &id
		var chatID *uuid.UUID

		chat, err := s.db.GetChatByUserIDs(r.Context(), database.GetChatByUserIDsParams{
			UserOneID: userID,
			UserTwoID: b.BlockedID,
		})
		if err == nil {
			cid := chat.ChatID
			chatID = &cid
			if chat.UserOneID == b.BlockedID { //te inició el chat: es anónimo para vos
				username = s.anonName(chat.ChatID)
				blockedID = nil
			}
		}
		blockedResponse = append(blockedResponse, blockResponse{
			BlockedID: blockedID,
			ChatID:    chatID,
			Username:  username,
			CreatedAt: b.CreatedAt,
		})
	}
	RespondWithJSON(w, 200, blockedResponse)
}

type reportRequest struct {
	Reason     string         `json:"reason"`
	Details    string 		  `json:"details"`
}
//readReport lee y valida el cuerpo. si algo está mal, ya respondió y devuelve false.
func readReport(w http.ResponseWriter, r *http.Request) (reportRequest, bool) {
	var body reportRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, 400, "error reading JSON")
		return body, false
	}
	switch body.Reason {
	case "spam", "acoso", "contenido_inapropiado":
		//válido, details es opcional
	case "otro":
		if body.Details == "" {
			RespondWithError(w, 400, "details are required when reason is 'otro'")
			return body, false
		}
	default:
		RespondWithError(w, 400, "invalid report reason")
		return body, false
	}
	return body, true
}
const maxReportsPerHour = 10
//saveReport guarda el report y responde sin devolver ningún ID de usuario.
func (s *state) saveReport(w http.ResponseWriter, r *http.Request, reporterID, reportedID uuid.UUID, chatID *uuid.UUID, body reportRequest) {
	reported, err := s.db.GetUserByID(r.Context(), reportedID)
	if err != nil {
		RespondWithError(w, 404, "User not found")
		return
	}

	var chatParam uuid.NullUUID
	if chatID != nil {
		chatParam = uuid.NullUUID{UUID: *chatID, Valid: true}
	}
	//checkeo si el usuario ya hizo demasiados reportes en la última hora y si el usuario ya reportó al mismo usuario por la misma razón en el último tiempo
	recent, err := s.db.CountRecentReportsByReporter(r.Context(), reporterID)
	if err != nil {
		RespondWithError(w, 500, "couldn't check report limits")
		return
	}
	if recent >= maxReportsPerHour {
		RespondWithError(w, 429, "too many reports, try again later")
		return
	}

	alreadyReported, err := s.db.ExistsRecentReport(r.Context(), database.ExistsRecentReportParams{
		ReporterID: reporterID,
		ReportedID: reportedID,
		Reason:     body.Reason,
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't check duplicate reports")
		return
	}
	if alreadyReported {
		RespondWithError(w, 409, "you already reported this user for this reason recently")
		return
	}
	report, err := s.db.CreateReportWithEvidence(r.Context(), database.CreateReportWithEvidenceParams{
		ReporterID:       reporterID,
		ReportedID:       reportedID,
		ChatID:           chatParam,
		ReportedUsername: reported.Username,
		Reason:           body.Reason,
		Details:          sql.NullString{String: body.Details, Valid: body.Details != ""},
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't create report")
		return
	}
	RespondWithJSON(w, 201, map[string]any{
		"report_id":  report.ReportID,
		"created_at": report.CreatedAt,
	})
}

func (s *state) reportUser(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	reportedUserID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		RespondWithError(w, 404, "Invalid user ID")
		return
	}
	body, ok := readReport(w, r)
	if !ok {
		return
	}
	s.saveReport(w, r, userID, reportedUserID, nil, body)
}

func (s *state) reportChat(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	chatID, err := uuid.Parse(r.PathValue("chatID"))
	if err != nil {
		RespondWithError(w, 404, "Invalid chat ID")
		return
	}
	otherID, err := s.otherParticipant(r.Context(), chatID, userID)
	if err != nil {
		respondChatError(w, err)
		return
	}
	body, ok := readReport(w, r)
	if !ok {
		return
	}
	s.saveReport(w, r, userID, otherID, &chatID, body)
}

func (s *state) getPublicProfile(w http.ResponseWriter, r *http.Request) {
	requesterID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	userIDString := r.PathValue("userID")
	userID, err := uuid.Parse(userIDString)
	if err != nil {
		RespondWithError(w, 404, "Invalid user ID")
		return
	}

	// tu propio perfil siempre se puede ver; el de otros, con reglas
	if requesterID != userID {
		blocked, err := s.isBlocked(r.Context(), requesterID, userID)
		if err != nil {
			RespondWithError(w, 500, "error checking block status")
			return
		}
		if blocked {
			RespondWithError(w, 403, "profile not available")
			return
		}
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		RespondWithError(w, 404, "User not found")
		return
	}

	photos, err := s.db.GetUserPhotos(r.Context(), userID)
	if err != nil {
		RespondWithError(w, 500, "couldn't get photos")
		return
	}

	photoURLs := []string{}
	for _, photo := range photos {
		photoURLs = append(photoURLs, absoluteURL(r, photo.PhotoUrl))
	}

	type responseProfile struct {
		UserID   uuid.UUID `json:"user_id"`
		Username string    `json:"username"`
		Bio      string    `json:"bio"`
		Photos   []string  `json:"photos"`
	}

	RespondWithJSON(w, 200, responseProfile{
		UserID:   user.UserID,
		Username: user.Username,
		Bio:      user.Bio.String,
		Photos:   photoURLs,
	})
}

func (s *state) uploadFile(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	err = r.ParseMultipartForm(10 << 20) // 10 MB máximo para el contenido
	if err != nil {
		RespondWithError(w, 400, "couldn't parse the uploaded file")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		RespondWithError(w, 400, "no file provided")
		return
	}
	defer file.Close()

	filename := userID.String() + "-" + header.Filename
	dst, err := os.Create("./uploads/" + filename)
	if err != nil {
		RespondWithError(w, 500, "couldn't save the file")
		return
	}
	defer dst.Close()
	_, err = io.Copy(dst, file)
	if err != nil {
		RespondWithError(w, 500, "couldn't save the file")
		return
	}

	fileURL := "/uploads/" + filename
	RespondWithJSON(w, 200, map[string]string{"url": fileURL})
}

type reorderPhotosRequest struct {
	PhotoIDs []uuid.UUID `json:"photo_ids"`
}

func (s *state) reorderPhotos(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	var body reorderPhotosRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, 400, "invalid request body")
		return
	}
	photos, err := s.db.GetUserPhotos(r.Context(), userID)
	if err != nil {
		RespondWithError(w, 500, "couldn't get photos")
		return
	}
	// la lista tiene que ser exactamente el conjunto de fotos del usuario
	owned := map[uuid.UUID]bool{}
	for _, p := range photos {
		owned[p.PhotoID] = true
	}
	if len(body.PhotoIDs) != len(photos) {
		RespondWithError(w, 400, "the list must include all of your photos")
		return
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range body.PhotoIDs {
		if !owned[id] || seen[id] {
			RespondWithError(w, 400, "invalid photo list")
			return
		}
		seen[id] = true
	}
	for i, id := range body.PhotoIDs {
		err := s.db.UpdatePhotoPosition(r.Context(), database.UpdatePhotoPositionParams{
			Position: int32(i),
			PhotoID:  id,
			UserID:   userID,
		})
		if err != nil {
			RespondWithError(w, 500, "couldn't update photo order")
			return
		}
	}
	RespondWithJSON(w, 200, map[string]string{"message": "photos reordered"})
}

func (s *state) getStoryViewers(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	storyIDString := r.PathValue("storyID")
	storyID, err := uuid.Parse(storyIDString)
	if err != nil {
		RespondWithError(w, 404, "Invalid story ID")
		return
	}
	story, err := s.db.GetStoryByID(r.Context(), storyID)
	if err != nil {
		RespondWithError(w, 404, "Story not found")
		return
	}
	if story.UserID != userID {
		RespondWithError(w, 403, "you can only see viewers of your own stories")
		return
	}
	viewers, err := s.db.GetStoryViewers(r.Context(), storyID)
	if err != nil {
		RespondWithError(w, 500, "couldn't get the viewers")
		return
	}
	type viewerResponse struct {
		ViewerID *uuid.UUID `json:"viewer_id,omitempty"`
		Username string    `json:"username"`
		ViewedAt time.Time `json:"viewed_at"`
	}
	viewersResponse := []viewerResponse{}
	for _, v := range viewers {
		if v.ViewerID == userID {
			continue
		}
		name := v.Username
		id := v.ViewerID
		viewerID := &id
		chat, err := s.db.GetChatByUserIDs(r.Context(), database.GetChatByUserIDsParams{
			UserOneID: userID,
			UserTwoID: v.ViewerID,
		})
		if err == nil && chat.UserOneID == v.ViewerID {
			name = s.anonName(chat.ChatID)
			viewerID = nil //espectador anónimo, no se devuelve su ID
		}
		viewersResponse = append(viewersResponse, viewerResponse{
			ViewerID: viewerID,
			Username: name,
			ViewedAt: v.ViewedAt,
		})
	}
	RespondWithJSON(w, 200, viewersResponse)
}

type updateDiscoveryRequest struct {
	GlobalDiscovery bool `json:"global_discovery"`
}

func (s *state) updateGlobalDiscovery(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	var body updateDiscoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, 400, "invalid request body")
		return
	}

	err = s.db.UpdateGlobalDiscovery(r.Context(), database.UpdateGlobalDiscoveryParams{
		GlobalDiscovery: body.GlobalDiscovery,
		UserID:          userID,
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't update your preference")
		return
	}

	RespondWithJSON(w, 200, map[string]bool{"global_discovery": body.GlobalDiscovery})
}

func (s *state) searchUsers(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" {
		RespondWithError(w, 400, "missing search query")
		return
	}
	users, err := s.db.SearchUsersByUsername(r.Context(), database.SearchUsersByUsernameParams{
		Query: escapeLike(query),
		UserID:  userID,
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't search users")
		return
	}
	type searchResult struct {
		UserID   uuid.UUID `json:"user_id"`
		Username string    `json:"username"`
		PhotoURL string    `json:"photo_url"`
	}
	results := []searchResult{}
	for _, u := range users {
		blocked, err := s.isBlocked(r.Context(), userID, u.UserID)
		if err != nil {
			RespondWithError(w, 500, "couldn't check blocks")
			return
		}
		if blocked {
			continue
		}
		results = append(results, searchResult{UserID: u.UserID, Username: u.Username, PhotoURL: absoluteURL(r, u.PhotoUrl)})
	}
	RespondWithJSON(w, 200, results)
}

type firstMessageRequest struct {
	Content string `json:"content"`
}

func (s *state) createFirstMessage(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	otherID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		RespondWithError(w, 400, "Invalid user ID")
		return
	}
	if otherID == userID {
		RespondWithError(w, 400, "you can't chat with yourself")
		return
	}

	var body firstMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, 400, "error reading JSON")
		return
	}
	content := strings.TrimSpace(body.Content)
	if content == "" {
		RespondWithError(w, 400, "message can't be empty")
		return
	}
	if _, err := s.db.GetUserByID(r.Context(), otherID); err != nil {
		RespondWithError(w, 404, "user not found")
		return
	}
	blocked, err := s.isBlocked(r.Context(), userID, otherID)
	if err != nil {
		RespondWithError(w, 500, "error checking block status")
		return
	}
	if blocked {
		RespondWithError(w, 403, "no se pueden enviar mensajes en este chat")
		return
	}
	chat, err := s.getOrCreateChat(r.Context(), userID, otherID)
	if err != nil {
		RespondWithError(w, 500, "couldn't get or create the chat")
		return
	}
	message, err := s.db.CreateMessage(r.Context(), database.CreateMessageParams{
		ChatID:   chat.ChatID,
		SenderID: userID,
		Content:  content,
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't create the message")
		return
	}
	
	go s.notifyNewMessage(chat.ChatID, userID, otherID, chat.UserOneID == otherID, content)
	s.emitNewMessage(chat.ChatID, otherID, wsMessage{
		MessageID: message.MessageID,
		Content:   message.Content,
		CreatedAt: message.CreatedAt,
	})
	RespondWithJSON(w, 201, map[string]any{
		"chat_id":    chat.ChatID,
		"message_id": message.MessageID,
	})
}

type deleteAccountRequest struct {
	Password string `json:"password"`
}

func (s *state) deleteAccount(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	var body deleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, 400, "error reading JSON")
		return
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		RespondWithError(w, 404, "User not found")
		return
	}

	rightPassword, err := auth.CheckPasswordHash(body.Password, user.PasswordHash)
	if err != nil || !rightPassword {
		RespondWithError(w, 403, "Incorrect password")
		return
	}
	// junto las rutas de archivos ANTES de borrar: el CASCADE se lleva las filas que las guardan
	files := []string{}
	photos, err := s.db.GetUserPhotos(r.Context(), userID)
	if err == nil {
		for _, p := range photos {
			files = append(files, p.PhotoUrl)
		}
	}
	stories, err := s.db.GetStoryMediaByUserID(r.Context(), userID)
	if err == nil {
		files = append(files, stories...)
	}
	// el CASCADE borra fotos, historias, chats, mensajes, fichas, bloqueos y tokens
	if err := s.db.DeleteUser(r.Context(), userID); err != nil {
		RespondWithError(w, 500, "couldn't delete the account")
		return
	}

	// recién ahora, borro los archivos del disco. un fallo acá no debe romper la respuesta
	for _, f := range files {
		removeUploadedFile(userID, f)
	}
	RespondWithJSON(w, 200, map[string]string{"status": "success"})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *state) changePassword(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	var body changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, 400, "error reading JSON")
		return
	}
	if len(body.NewPassword) < 6 {
		RespondWithError(w, 400, "new password must be at least 6 characters")
		return
	}
	if body.NewPassword == body.CurrentPassword {
		RespondWithError(w, 400, "new password must be different")
		return
	}
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		RespondWithError(w, 404, "User not found")
		return
	}
	rightPassword, err := auth.CheckPasswordHash(body.CurrentPassword, user.PasswordHash)
	if err != nil || !rightPassword {
		RespondWithError(w, 403, "Incorrect password")
		return
	}
	hashed, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		RespondWithError(w, 500, "error hashing the password")
		return
	}
	err = s.db.UpdateUserPassword(r.Context(), database.UpdateUserPasswordParams{
		UserID:       userID,
		PasswordHash: hashed,
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't update the password")
		return
	}
	// cierro todas las sesiones. si falla, la contraseña ya cambió, así que solo lo registro
	if err := s.db.RevokeAllUserRefreshTokens(r.Context(), userID); err != nil {
		log.Printf("couldn't revoke refresh tokens for %s: %v", userID, err)
	}
	RespondWithJSON(w, 200, map[string]string{"status": "success"})
}

type pushTokenRequest struct {
	Token string `json:"token"`
}

func validPushToken(t string) bool {
	return (strings.HasPrefix(t, "ExponentPushToken[") || strings.HasPrefix(t, "ExpoPushToken[")) &&
		strings.HasSuffix(t, "]")
}

func (s *state) savePushToken(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	var req pushTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondWithError(w, 400, "Error reading JSON")
		return
	}
	if !validPushToken(req.Token) {
		RespondWithError(w, 400, "Invalid push token")
		return
	}

	err = s.db.UpsertPushToken(r.Context(), database.UpsertPushTokenParams{
		Token:  req.Token,
		UserID: userID,
	})
	if err != nil {
		RespondWithError(w, 500, "Couldn't save push token")
		return
	}
	RespondWithJSON(w, 200, map[string]string{"status": "success"})
}

func (s *state) deletePushToken(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	var req pushTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		RespondWithError(w, 400, "Invalid request body")
		return
	}
	err = s.db.DeletePushTokenForUser(r.Context(), database.DeletePushTokenForUserParams{
		Token:  req.Token,
		UserID: userID,
	})
	if err != nil {
		RespondWithError(w, 500, "Couldn't delete push token")
		return
	}
	RespondWithJSON(w, 200, map[string]string{"status": "success"})
}
func (s *state) getPushMode(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	mode, err := s.db.GetPushMode(r.Context(), userID)
	if err != nil {
		RespondWithError(w, 500, "couldn't get notification settings")
		return
	}
	RespondWithJSON(w, 200, map[string]string{"mode": mode})
}

func (s *state) setPushMode(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, 400, "error reading JSON")
		return
	}
	switch body.Mode {
	case "off", "name", "full":
	default:
		RespondWithError(w, 400, "invalid mode")
		return
	}
	err = s.db.UpdatePushMode(r.Context(), database.UpdatePushModeParams{
		UserID:   userID,
		PushMode: body.Mode,
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't update notification settings")
		return
	}
	RespondWithJSON(w, 200, map[string]string{"mode": body.Mode})
}

func (s *state) blockChat(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	chatID, err := uuid.Parse(r.PathValue("chatID"))
	if err != nil {
		RespondWithError(w, 404, "Invalid chat ID")
		return
	}
	otherID, err := s.otherParticipant(r.Context(), chatID, userID)
	if err != nil {
		respondChatError(w, err)
		return
	}
	_, err = s.db.CreateBlock(r.Context(), database.CreateBlockParams{
		BlockerID: userID,
		BlockedID: otherID,
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't block user")
		return
	}
	RespondWithJSON(w, 200, map[string]interface{}{"status": "success"})
}

func (s *state) unblockChat(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	chatID, err := uuid.Parse(r.PathValue("chatID"))
	if err != nil {
		RespondWithError(w, 404, "Invalid chat ID")
		return
	}
	otherID, err := s.otherParticipant(r.Context(), chatID, userID)
	if err != nil {
		respondChatError(w, err)
		return
	}
	err = s.db.DeleteBlockByBlockerAndBlockedID(r.Context(), database.DeleteBlockByBlockerAndBlockedIDParams{
		BlockerID: userID,
		BlockedID: otherID,
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't unblock user")
		return
	}
	RespondWithJSON(w, 200, map[string]interface{}{"status": "success"})
}

type storyReplyRequest struct {
	Content string `json:"content"`
}

func (s *state) replyToStory(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	userID, err := s.getUserIDFromContext(r)
	if err != nil {
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	storyID, err := uuid.Parse(r.PathValue("storyID"))
	if err != nil {
		RespondWithError(w, 404, "Invalid story ID")
		return
	}
	var body storyReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, 400, "error reading JSON")
		return
	}
	content := strings.TrimSpace(body.Content)
	if content == "" {
		RespondWithError(w, 400, "message can't be empty")
		return
	}

	story, err := s.db.GetStoryByID(r.Context(), storyID)
	if err != nil || time.Now().UTC().After(story.ExpiresAt) {
		RespondWithError(w, 404, "Story not found")
		return
	}
	if story.UserID == userID {
		RespondWithError(w, 403, "Can't reply to your own story")
		return
	}
	blocked, err := s.isBlocked(r.Context(), userID, story.UserID)
	if err != nil {
		RespondWithError(w, 500, "error checking block status")
		return
	}
	if blocked {
		RespondWithError(w, 403, "Can't reply to this story")
		return
	}
	
	chat, err := s.getOrCreateChat(r.Context(), userID, story.UserID)
	if err != nil {
		RespondWithError(w, 500, "couldn't get or create the chat")
		return
	}
	message, err := s.db.CreateStoryReplyMessage(r.Context(), database.CreateStoryReplyMessageParams{
		ChatID:   chat.ChatID,
		SenderID: userID,
		Content:  content,
		StoryID:  uuid.NullUUID{UUID: storyID, Valid: true},
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't create the message")
		return
	}

	go s.notifyNewMessage(chat.ChatID, userID, story.UserID, chat.UserOneID == story.UserID, "Respondió a tu historia: "+content)
	mediaURL := absoluteURL(r, story.MediaUrl)
	mediaType := story.MediaType
	s.emitNewMessage(chat.ChatID, story.UserID, wsMessage{
		MessageID: message.MessageID,
		Content:   message.Content,
		CreatedAt: message.CreatedAt,
		IsStoryReply:   true,
		StoryMediaURL:  &mediaURL,
		StoryMediaType: &mediaType,
	})
	RespondWithJSON(w, 201, map[string]any{"chat_id":    chat.ChatID,"message_id": message.MessageID,})
}

func (s *state) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, 400, "Pedido inválido")
		return
	}
	email := strings.TrimSpace(body.Email)

	//la respuesta es SIEMPRE esta, exista o no el email
	respondOK := func() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Si el email está registrado, te enviamos un código",
		})
	}

	user, err := s.db.GetUserByEmail(r.Context(), email)
	if errors.Is(err, sql.ErrNoRows) {
		respondOK() //no existe: no lo revelamos
		return
	}
	if err != nil {
		RespondWithError(w, 500, "Error interno")
		return
	}

	//enfriamiento: si ya mandamos un código hace menos de 60 s, no mandamos otro
	existing, err := s.db.GetPasswordReset(r.Context(), user.UserID)
	if err == nil && time.Since(existing.CreatedAt) < 60*time.Second {
		respondOK()
		return
	}

	code, err := generateResetCode()
	if err != nil {
		RespondWithError(w, 500, "Error interno")
		return
	}
	err = s.db.UpsertPasswordReset(r.Context(), database.UpsertPasswordResetParams{
		UserID:    user.UserID,
		CodeHash:  hashResetCode(code),
		ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
	})
	if err != nil {
		RespondWithError(w, 500, "Error interno")
		return
	}

	//el mail se manda en segundo plano para no estar tanto tiempo y terminar develando que mail usa la aplicacion.
	go func() {
		body := "Tu código para recuperar la contraseña es: " + code +
			"\n\nVence en 15 minutos. Si no lo pediste vos, ignorá este mail."
		if err := s.mailer.Send(user.Email, "Recuperar contraseña", body); err != nil {
			log.Printf("no se pudo enviar el mail de recuperación: %v", err)
		}
	}()

	respondOK()
}

const maxResetAttempts = 5

type resetPasswordRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

func (s *state) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondWithError(w, 400, "error reading JSON")
		return
	}
	email := strings.TrimSpace(body.Email)
	code := strings.TrimSpace(body.Code)
	if len(body.NewPassword) < 6 {
		RespondWithError(w, 400, "new password must be at least 6 characters")
		return
	}

	//todas las fallas "del código" responden lo mismo, sin aclarar cuál fue
	invalid := func() {
		RespondWithError(w, 400, "Código inválido o vencido")
	}

	user, err := s.db.GetUserByEmail(r.Context(), email)
	if errors.Is(err, sql.ErrNoRows) {
		invalid()
		return
	}
	if err != nil {
		RespondWithError(w, 500, "Error interno")
		return
	}

	reset, err := s.db.GetPasswordReset(r.Context(), user.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		invalid()
		return
	}
	if err != nil {
		RespondWithError(w, 500, "Error interno")
		return
	}

	//venció o ya se gastaron los intentos, el código no sirve más
	if time.Now().UTC().After(reset.ExpiresAt) || reset.Attempts >= maxResetAttempts {
		s.db.DeletePasswordReset(r.Context(), user.UserID)
		invalid()
		return
	}

	//comparación en tiempo constante, una comparación normal se detiene en el primer carácter distinto, así que tarda un poco menos cuando el primer dígito falla. 
	//midiendo tiempos se podría adivinar el código de a un carácter, la función de subtle siempre tarda lo mismo.
	if subtle.ConstantTimeCompare([]byte(hashResetCode(code)), []byte(reset.CodeHash)) != 1 {
		s.db.IncrementResetAttempts(r.Context(), user.UserID)
		invalid()
		return
	}

	hashed, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		RespondWithError(w, 500, "error hashing the password")
		return
	}
	err = s.db.UpdateUserPassword(r.Context(), database.UpdateUserPasswordParams{
		UserID:       user.UserID,
		PasswordHash: hashed,
	})
	if err != nil {
		RespondWithError(w, 500, "couldn't update the password")
		return
	}

	//el código ya se usó: se borra para que no sirva dos veces
	s.db.DeletePasswordReset(r.Context(), user.UserID)

	//cortar todas las sesiones, igual que en change-password
	if err := s.db.RevokeAllUserRefreshTokens(r.Context(), user.UserID); err != nil {
		log.Printf("couldn't revoke refresh tokens for %s: %v", user.UserID, err)
	}
	s.hub.disconnectUser(user.UserID)

	RespondWithJSON(w, 200, map[string]string{"status": "success"})
}


//el state es el receiver, (el objeto que está ejecutando el método)
//cuando handlers() escribe s.register, ese s es el mismo que le llegó a handlers(),  necesita recibir la instancia de alguna manera 
func (s *state) handlers() {
	loginLimiter := newRateLimiter(12*time.Second, 5)   //5 intentos seguidos, luego 1 cada 12 s
	registerLimiter := newRateLimiter(time.Minute, 3)   //3 registros seguidos, luego 1 por minuto

	//handle toma si o si del tipo handler, y como s.register no lo es, la convertimos con handlerfunc
	http.Handle("POST /login", loginLimiter.middleware(http.HandlerFunc(s.login)))
	http.Handle("POST /register", registerLimiter.middleware(http.HandlerFunc(s.register)))
	http.Handle("POST /refresh", http.HandlerFunc(s.refresh))
	http.Handle("POST /logout", http.HandlerFunc(s.logout))

	http.Handle("POST /chats/{chatID}/messages", s.authMiddleware(http.HandlerFunc(s.createMessage)))
	http.Handle("GET /chats/{chatID}/messages", s.authMiddleware(http.HandlerFunc(s.getMessages)))
	http.Handle("DELETE /chats/{chatID}", s.authMiddleware(http.HandlerFunc(s.deleteChat)))
	
	http.Handle("PATCH /me", s.authMiddleware(http.HandlerFunc(s.updateFields)))
	http.Handle("GET /chats/{chatID}/card", s.authMiddleware(http.HandlerFunc(s.getUserCard)))
	http.Handle("GET /chats", s.authMiddleware(http.HandlerFunc(s.getChats)))

	http.Handle("PATCH /chats/{chatID}/card/nickname", s.authMiddleware(http.HandlerFunc(s.updateNickname)))
	http.Handle("PATCH /chats/{chatID}/card/notes", s.authMiddleware(http.HandlerFunc(s.updateNotesOnSubject)))
	http.Handle("POST /chats/{chatID}/card/reveal/{field}", s.authMiddleware(http.HandlerFunc(s.revealAField)))
	http.Handle("POST /chats/{chatID}/card/reveal-all", s.authMiddleware(http.HandlerFunc(s.revealAllFields)))

	http.Handle("PATCH /chats/{chatID}/card/reset", s.authMiddleware(http.HandlerFunc(s.resetCard)))

	http.Handle("GET /me", s.authMiddleware(http.HandlerFunc(s.getMe)))
	//s.getMe es un handler, pero no cumple la interfaz http.Handler. entonces lo envolvemos en http.HandlerFunc para que cumpla la interfaz.
	//s.authMiddleware es un middleware que toma un handler y devuelve un handler. entonces le pasamos el handler envuelto en http.HandlerFunc, y nos devuelve otro handler que valida el JWT antes de ejecutar s.getMe.
	//entonces, cuando llega una request a /me, primero pasa por s.authMiddleware, que valida el JWT y pone el userID en el contexto de la request, y luego ejecuta s.getMe con ese contexto.
	//s.getMe puede acceder al userID del contexto de la request gracias a s.authMiddleware. si no hubiera pasado por el middleware, s.getMe no podría acceder al userID y devolvería un error 401.
	//resumen: s.getMe (tu handler) → envuelto en http.HandlerFunc (para que cumpla la interfaz http.Handler que authMiddleware espera recibir) → envuelto en s.authMiddleware (que valida el JWT antes de dejarlo pasar) → registrado con http.Handle.

	http.Handle("POST /me/photos", s.authMiddleware(http.HandlerFunc(s.uploadPhoto)))
	http.Handle("GET /me/photos", s.authMiddleware(http.HandlerFunc(s.getPhotos)))
	http.Handle("DELETE /me/photos/{photoID}", s.authMiddleware(http.HandlerFunc(s.deletePhoto)))

	http.Handle("GET /users/discover", s.authMiddleware(http.HandlerFunc(s.discoverUsers)))

	http.Handle("POST /me/stories", s.authMiddleware(http.HandlerFunc(s.createStory)))
	http.Handle("GET /stories", s.authMiddleware(http.HandlerFunc(s.getActiveStories)))
	http.Handle("POST /stories/{storyID}/view", s.authMiddleware(http.HandlerFunc(s.viewStory)))
	http.Handle("DELETE /stories/{storyID}", s.authMiddleware(http.HandlerFunc(s.deleteStory)))
	http.Handle("GET /stories/{storyID}/viewers", s.authMiddleware(http.HandlerFunc(s.getStoryViewers)))

	http.Handle("POST /me/block/{userID}", s.authMiddleware(http.HandlerFunc(s.blockUser)))
	http.Handle("DELETE /me/unblock/{userID}", s.authMiddleware(http.HandlerFunc(s.unblockUser)))
	http.Handle("GET /me/blocked", s.authMiddleware(http.HandlerFunc(s.getBlockedUsers)))
	http.Handle("POST /me/report/{userID}", s.authMiddleware(http.HandlerFunc(s.reportUser)))

	http.Handle("GET /users/{userID}/profile", s.authMiddleware(http.HandlerFunc(s.getPublicProfile)))

	http.Handle("POST /upload", s.authMiddleware(http.HandlerFunc(s.uploadFile)))
	http.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./uploads"))))
	http.Handle("PATCH /me/photos/order", s.authMiddleware(http.HandlerFunc(s.reorderPhotos)))

	http.Handle("PATCH /me/discovery-settings", s.authMiddleware(http.HandlerFunc(s.updateGlobalDiscovery)))
	http.Handle("GET /users/search", s.authMiddleware(http.HandlerFunc(s.searchUsers)))
	http.Handle("POST /users/{userID}/messages", s.authMiddleware(http.HandlerFunc(s.createFirstMessage)))

	http.Handle("DELETE /me", s.authMiddleware(http.HandlerFunc(s.deleteAccount)))
	http.Handle("PATCH /me/password", s.authMiddleware(http.HandlerFunc(s.changePassword)))

	http.Handle("POST /me/push-token", s.authMiddleware(http.HandlerFunc(s.savePushToken)))
	http.Handle("DELETE /me/push-token", s.authMiddleware(http.HandlerFunc(s.deletePushToken)))
	http.Handle("GET /me/push-mode", s.authMiddleware(http.HandlerFunc(s.getPushMode)))
	http.Handle("PATCH /me/push-mode", s.authMiddleware(http.HandlerFunc(s.setPushMode)))

	http.Handle("POST /chats/{chatID}/block", s.authMiddleware(http.HandlerFunc(s.blockChat)))
	http.Handle("DELETE /chats/{chatID}/block", s.authMiddleware(http.HandlerFunc(s.unblockChat)))
	http.Handle("POST /chats/{chatID}/report", s.authMiddleware(http.HandlerFunc(s.reportChat)))

	http.Handle("POST /stories/{storyID}/reply", s.authMiddleware(http.HandlerFunc(s.replyToStory)))

	http.HandleFunc("GET /ws", s.wsHandler) //para websockets no necesitamos authMiddleware porque la autenticación se hace dentro de wsHandler, no antes.

	forgotLimiter := newRateLimiter(time.Minute, 3)
	http.Handle("POST /forgot-password", forgotLimiter.middleware(http.HandlerFunc(s.forgotPassword)))
	resetLimiter := newRateLimiter(12*time.Second, 5)
	http.Handle("POST /reset-password", resetLimiter.middleware(http.HandlerFunc(s.resetPassword)))
}




