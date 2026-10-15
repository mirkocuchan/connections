#!/bin/bash
PORT=8080
TOKEN=$(curl -s -X POST localhost:$PORT/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"mirko@gmail","password":"mirko"}' | jq -r .token)
echo "token: ${TOKEN:0:20}..."
npx wscat -c "ws://localhost:$PORT/ws?token=$TOKEN"
