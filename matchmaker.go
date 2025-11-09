package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/genai"
)

type Client struct {
	email  string
	elo    int
	swipes map[*websocket.Conn]int
	socket *websocket.Conn
	match  *websocket.Conn
}

var emails = make(map[string]*websocket.Conn)
var clients = make(map[*websocket.Conn]*Client)
var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

const apiKey = "AIzaSyA9827kIbM80sKDjfilQAXLLfFh0UZvmjk"

func get_rating(message string) string {
	ctx := context.Background()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal(err)
	}

	my := int32(0)
	result, err := client.Models.GenerateContent(ctx,
		"gemini-2.0-flash",
		genai.Text(`Choose one of the following words (YOU SHOULD ONLY RETURN A SINGLE WORD NO OTHER TEXT), [brilliant, great, good, mistake, blunder] which bests describes this message in the context of an online dating conversation. If it is somewhat regular then return an empty string. Message: `+message),
		&genai.GenerateContentConfig{
			MaxOutputTokens: 10,
			ThinkingConfig: &genai.ThinkingConfig{
				ThinkingBudget: &my,
			},
		},
	)

	return strings.ToLower(result.Text())
}

func main() {
	log.Println("Server running at 8080")

	go Matcher()

	http.HandleFunc("/ws", Connect)
	http.ListenAndServe(":8080", nil)
}

func Connect(w http.ResponseWriter, r *http.Request) {
	socket, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Println("Error upgrading:", err)
	} else {
		log.Println("Successful connection")
		_, email, err := socket.ReadMessage()

		if err != nil {
			log.Println(err)
			return
		}

		_, elo, err := socket.ReadMessage()

		elo_int, err := strconv.Atoi(string(elo))
		email_str := string(email)

		val, ok := emails[email_str]

		if ok {
			delete(clients, val)
		}

		log.Println(err)
		if err == nil {
			log.Println("New client created: ", elo_int, email_str)

			client := &Client{
				email:  email_str,
				elo:    elo_int,
				socket: socket,
				swipes: make(map[*websocket.Conn]int),
				match:  nil,
			}
			clients[socket] = client
			emails[email_str] = socket

			go client.GetMessages()
		} else {
			log.Println(err)
		}
	}
}

func (client *Client) GetMessages() {
	defer delete(clients, client.socket)
	defer client.socket.Close()

	for {
		_, message, err := client.socket.ReadMessage()
		if err != nil {
			break
		}
		if client.match != nil {
			Send(string(message), client.match, client.email)
			Send(string(message), client.socket, client.email)
		}
	}
}

func Matcher() {
	var unmatched []*websocket.Conn

	for socket, client := range clients {
		if client.match == nil {
			unmatched = append(unmatched, socket)
		}
	}

	sort.Slice(unmatched, func(i, j int) bool {
		return clients[unmatched[i]].elo < clients[unmatched[j]].elo
	})

	n := len(unmatched)
	if n > 1 {
		for i := 0; i < n; i += 2 {
			if i+1 >= n {
				return
			} else {
				socket_1 := unmatched[i]
				socket_2 := unmatched[i+1]

				log.Println("Found a match: ", clients[socket_1].email, clients[socket_2].email)

				clients[socket_1].Match(socket_2)
				clients[socket_2].Match(socket_1)

				log.Println(clients[socket_1].match == nil)
			}
		}
	}

	time.Sleep(time.Second)

	Matcher()
}

func (client *Client) Match(socket *websocket.Conn) {
	client.match = socket

	log.Println("Sending email")
	err := client.socket.WriteMessage(websocket.TextMessage, []byte(clients[socket].email))
	if err != nil {
		return
	}
}

func Send(message string, socket *websocket.Conn, author string) {
	log.Println("Sending message")
	err := socket.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(message+"~"+author+"~"+get_rating(message))))
	if err != nil {
		return
	}
}
