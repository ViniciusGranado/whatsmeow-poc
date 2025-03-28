package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/glebarez/sqlite"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type DeepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func callDeepSeek(message string) {
	url := "https://api.deepseek.com/chat/completions"
	method := "POST"

	requestBody := map[string]interface{}{
		"messages": []DeepSeekMessage{
			{Role: "system", Content: "You are a helpful assistant"},
			{Role: "user", Content: message},
		},
		"model":             "deepseek-chat",
		"frequency_penalty": 0,
		"max_tokens":        2048,
		"presence_penalty":  0,
		"response_format": map[string]string{
			"type": "text",
		},
		"stop":           nil,
		"stream":         false,
		"stream_options": nil,
		"temperature":    1,
		"top_p":          1,
		"tools":          nil,
		"tool_choice":    "none",
		"logprobs":       false,
		"top_logprobs":   nil,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		fmt.Println("JSON marshal error:", err)
		return
	}

	client := &http.Client{}
	req, err := http.NewRequest(method, url, bytes.NewReader(jsonData))

	if err != nil {
		fmt.Println(err)
		return
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer")

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(body))
}

type Conversation struct {
	gorm.Model
	JID  string `gorm:"uniqueIndex"`
	Name string
}

type Message struct {
	gorm.Model
	JID      string `gorm:"column:jid"`
	Message  string
	IsFromMe bool
	OrderId  uint64
}

var db, err = gorm.Open(sqlite.Open("whatsmeow-poc.db"), &gorm.Config{})

func createConversation(JID string, name string) Conversation {
	newConversation := Conversation{
		JID:  JID,
		Name: name,
	}

	res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&newConversation)

	if res.Error != nil {
		panic(res.Error)
	}

	return newConversation
}

func createMessage(JID string, message string, isFromMe bool, orderId uint64) Message {
	newMessage := Message{
		JID:      JID,
		Message:  message,
		IsFromMe: isFromMe,
		OrderId:  orderId,
	}

	res := db.Create(&newMessage)

	if res.Error != nil {
		panic(res.Error)
	}

	return newMessage
}

func getMessages(JID string) []Message {
	var messages []Message

	res := db.Where("jid = ?", JID).Order("order_id").Find(&messages)

	if res.Error != nil {
		panic(res.Error)
	}

	return messages
}

func main() {
	db.AutoMigrate(&Conversation{}, &Message{})

	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}
	// clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, nil)

	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		// case *events.Message:
		// 	fmt.Println("Received a message!", v.Message.GetConversation())
		// 	fmt.Println(v.Info.Chat.String())
		// case *events.Connected:
		// 	fmt.Println("Connected to WhatsApp!")
		case *events.HistorySync:
			fmt.Println("History sync")
			for _, conversation := range v.Data.Conversations {
				if *conversation.ID == "5519983921932@s.whatsapp.net" {
					var name string
					if conversation.Name != nil {
						name = *conversation.Name
					}

					createConversation(*conversation.ID, name)

					for _, message := range conversation.Messages {
						fmt.Println(*message.MsgOrderID)
						fmt.Println(*message.Message.Message.ExtendedTextMessage.Text)
						fmt.Println(*message.Message.Key.FromMe)

						createMessage(*conversation.ID, *message.Message.Message.ExtendedTextMessage.Text, *message.Message.Key.FromMe, *message.MsgOrderID)
					}

					// jid, _ := types.ParseJID(*conversation.ID)
					// m1, _ := client.ParseWebMessage(jid, conversation.Messages[len(conversation.Messages)-1].Message)
					// build := client.BuildHistorySyncRequest(&m1.Info, 50)
					// client.SendMessage(context.Background(), jid, build)

					if len(conversation.Messages) == 0 {
						messages := getMessages(*conversation.ID)

						var builder strings.Builder
						builder.WriteString("Please give me a summary of this messages. Please give me the result in portuguese:\\n")

						for _, message := range messages {
							fmt.Println(message.ID)
							fmt.Println(message.Message)

							if message.IsFromMe {
								builder.WriteString("Me: ")
							} else {
								builder.WriteString("Other: ")
							}

							builder.WriteString(message.Message + "\\n")
						}

						callDeepSeek(builder.String())
					}
				}
			}

		}
	})

	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal
				fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}
