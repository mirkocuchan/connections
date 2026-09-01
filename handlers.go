package main

import(
	"net/http"
	"io"
	"github.com/mirkocuchan/connections/internal/auth"
	"github.com/mirkocuchan/connections/internal/database"
	"time"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"database/sql"
)

type receivedUser struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DateOfBirth Date   `json:"date_of_birth"`
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
	})
}

type createChatRequest struct {
    OtherUserID uuid.UUID `json:"other_user_id"`
}

func (s *state) createChat(w http.ResponseWriter, r *http.Request){
	defer r.Body.Close()

	userData, err := io.ReadAll(r.Body)
	if err != nil{
		RespondWithError(w, 400, "couldn't read the request body")
		return
	}
	
	var body createChatRequest
	if err := json.Unmarshal(userData, &body); err != nil {
        RespondWithError(w, 400, "error unmarshalling JSON")
		return
    }
	//getting the userID of the user that is being talked to

	//quiero obtener el userID del contexto de la request. el middleware authMiddleware lo puso ahí, así que si llegamos a este punto, el userID debería estar en el contexto.
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	userIDParams := database.GetChatByUserIDsParams{
		UserOneID: userID,
		UserTwoID: body.OtherUserID, // Reemplaza con el userID del otro usuario
	}

	chat, err := s.db.GetChatByUserIDs(r.Context(), userIDParams)
	
	if err == sql.ErrNoRows{
		//si no hay chat, lo creo. si hay chat, devuelvo el chat existente.
		newChatParams := database.CreateChatParams{
			UserOneID: userID,
			UserTwoID: body.OtherUserID, // Reemplaza con el userID del otro usuario
		}

		newChat, err := s.db.CreateChat(r.Context(), newChatParams)
		if err != nil{
			RespondWithError(w, 500, "couldn't create the chat in the database")
			return
		}
			
		RespondWithJSON(w, 200, map[string]string{"message": "chat created successfully", "chat_id": newChat.ChatID.String()})
		return
	}
	
	if err != nil{
		RespondWithError(w, 500, "couldn't find the chat in the database")
		return
	}

	RespondWithJSON(w, 200, map[string]string{"message": "chat already exists", "chat_id": chat.ChatID.String()})
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

	type messageResponse struct {
		MessageID uuid.UUID `json:"message_id"`
		ChatID    uuid.UUID `json:"chat_id"`
		SenderID  uuid.UUID `json:"sender_id"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	RespondWithJSON(w, 201, messageResponse{
		MessageID: newMessage.MessageID,
		ChatID:    newMessage.ChatID,
		SenderID:  newMessage.SenderID,
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
		SenderID  uuid.UUID `json:"sender_id"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	messagesResponse := []messageResponse{}
	for _, message := range messages{
		messagesResponse = append(messagesResponse, messageResponse{
			MessageID: message.MessageID,
			ChatID:    message.ChatID,
			SenderID:  message.SenderID,
			Content:   message.Content,
			CreatedAt: message.CreatedAt,
			UpdatedAt: message.UpdatedAt,})
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
	
	//cardParams
	cardParams := database.GetCardWithChatCreatorAndSubjectParams{
		ChatID: chatID,
		CreatorID: userID,
		SubjectID: subjectID,
	}
	type responseCard struct {
		CardID             uuid.UUID `json:"card_id"`
		ChatID             uuid.UUID `json:"chat_id"`
		CreatorID          uuid.UUID `json:"creator_id"`
		SubjectID          uuid.UUID `json:"subject_id"`
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
			
		RespondWithJSON(w, 200, responseCard{CardID: newCard.CardID, ChatID: chat.ChatID, CreatorID: newCard.CreatorID, SubjectID: newCard.SubjectID, Nickname: newCard.Nickname,
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
	
	RespondWithJSON(w, 200, responseCard{CardID: card.CardID, ChatID: chat.ChatID, CreatorID: card.CreatorID, SubjectID: card.SubjectID, Nickname: card.Nickname,
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
		CreatorID          uuid.UUID `json:"creator_id"`
		SubjectID          uuid.UUID `json:"subject_id"`
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
	RespondWithJSON(w, 200, responseCard{CardID: updatedCard.CardID, ChatID: updatedCard.ChatID, CreatorID: updatedCard.CreatorID,         
		SubjectID: updatedCard.SubjectID, Nickname: updatedCard.Nickname, NotesOnSubject: updatedCard.NotesOnSubject, DisplayNameVisible: updatedCard.DisplayNameVisible, DateOfBirthVisible: updatedCard.DateOfBirthVisible,
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
		CreatorID          uuid.UUID `json:"creator_id"`
		SubjectID          uuid.UUID `json:"subject_id"`
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
	RespondWithJSON(w, 200, responseCard{CardID: updatedCard.CardID, ChatID: updatedCard.ChatID, CreatorID: updatedCard.CreatorID,         
		SubjectID: updatedCard.SubjectID, Nickname: updatedCard.Nickname, NotesOnSubject: updatedCard.NotesOnSubject, DisplayNameVisible: updatedCard.DisplayNameVisible, DateOfBirthVisible: updatedCard.DateOfBirthVisible,
		CityVisible: updatedCard.CityVisible, CountryVisible: updatedCard.CountryVisible, PhotosVisible: updatedCard.PhotosVisible, BioVisible: updatedCard.BioVisible, HobbiesVisible: updatedCard.HobbiesVisible,
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
	//userID's chats
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}

	chats, err := s.db.GetChatsByUserID(r.Context(), userID)
	if err != nil{
		RespondWithError(w, 404, "chat doesn't exist in the database")
    	return
	}

	type chatResponse struct {
		ChatID    uuid.UUID `json:"chat_id"`
		UserOneID uuid.UUID `json:"creator_id"`
		UserTwoID uuid.UUID    `json:"subject_id"`
		Nickname string `json:"nickname"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
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
		displayName := "anon-" + otherUserID.String()[:8]  // placeholder por default
		if err == nil && card.Nickname.Valid {
			displayName = card.Nickname.String
		}
		chatsResponse = append(chatsResponse, chatResponse{
        ChatID:    chat.ChatID,
        UserOneID: chat.UserOneID,
        UserTwoID: chat.UserTwoID,
        Nickname:  displayName,
        CreatedAt: chat.CreatedAt,
        UpdatedAt: chat.UpdatedAt,
    	})
	}

	RespondWithJSON(w, 200, chatsResponse)
}

type createPhotoStruct struct {
	PhotoData string `json:"photo_data"`
	Position int `json:"position"`
}
func (s *state) createPhoto(w http.ResponseWriter, r *http.Request){
	defer r.Body.Close()

	imageData, err := io.ReadAll(r.Body)
	if err != nil{
		RespondWithError(w, 400, "couldn't read the request body")
		return
	}
	
	var body createPhotoStruct
	if err := json.Unmarshal(messageData, &body); err != nil {
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
		PhotoUrl: body.PhotoData,
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
		UpdatedAt time.Time `json:"updated_at"`
	}
	RespondWithJSON(w, 200, responsePhoto{PhotoID: userPhoto.PhotoID, UserID: userPhoto.UserID, PhotoUrl: userPhoto.PhotoUrl, Position: userPhoto.Position, CreatedAt: userPhoto.CreatedAt, UpdatedAt: userPhoto.UpdatedAt})
}
//el state es el receiver, (el objeto que está ejecutando el método)
//cuando handlers() escribe s.register, ese s es el mismo que le llegó a handlers(),  necesita recibir la instancia de alguna manera 
func (s *state) handlers() {
	//handle toma si o si del tipo handler, y como s.register no lo es, la convertimos con handlerfunc
	http.Handle("POST /register", http.HandlerFunc(s.register))
	http.Handle("POST /login", http.HandlerFunc(s.login))
	http.Handle("POST /refresh", http.HandlerFunc(s.refresh))
	http.Handle("POST /logout", http.HandlerFunc(s.logout))

	http.Handle("POST /chats", s.authMiddleware(http.HandlerFunc(s.createChat)))
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
}
