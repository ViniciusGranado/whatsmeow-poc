package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"github.com/mdp/qrterminal/v3"
	_ "github.com/mattn/go-sqlite3"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func main() {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			fmt.Println("Received a message!", v.Message.GetConversation())
			fmt.Println(v.Info.Chat.String())
		case *events.Connected:
			fmt.Println("Connected to WhatsApp!")
		// case *events.AppStateSyncComplete:
		// 	// Fetch contacts from the device's chat store
		// 	contacts, err := deviceStore.Contacts.GetAllContacts()

		// 	if err != nil {
		// 		fmt.Println("Failed to fetch chats:", err)
		// 		return
		// 	}
		// 	fmt.Println("\n--- All Chats ---")
		// 	for _, contact := range contacts {
		// 		if contact.FullName != "" {
		// 			fmt.Printf("- %s (%s) %s\n", contact.FullName, contact.PushName, contact.)
		// 		}
		// 	}
		// 	fmt.Println("-----------------")
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
