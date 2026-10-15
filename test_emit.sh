#!/bin/bash
PORT=8080
JC_EMAIL="jc@gmail"
JC_PASS="jc"
CHAT_ID=54fb16ad-43e5-4e6b-b9bc-b43c607d9ac6
TOKEN=$(curl -s -X POST localhost:$PORT/login -H 'Content-Type: application/json' \
  -d "{\"email\":\"$JC_EMAIL\",\"password\":\"$JC_PASS\"}" | jq -r .token)
curl -s -X POST localhost:$PORT/chats/$CHAT_ID/messages \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"content":"hola desde el socket"}' | jq
