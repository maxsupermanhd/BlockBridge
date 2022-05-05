package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/chat"
	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
	"github.com/maxsupermanhd/WebChunk/credentials"
	"github.com/natefinch/lumberjack"
)

type botConf struct {
	ServerAddress   string
	DiscordToken    string
	AllowedSlash    []string
	SpamInterval    int
	SpamMessage     chat.Message
	LogsFilename    string
	LogsMaxSize     int
	CredentialsRoot string
	MCUsername      string
	ChannelID       string
}

var loadedConfig botConf

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func loadConfig() {
	path := os.Getenv("BLOCKBRIDGE_CONFIG")
	if path == "" {
		path = "config.json"
	}
	b, err := os.ReadFile(path)
	must(err)
	err = json.Unmarshal(b, &loadedConfig)
	must(err)
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	loadConfig()
	log.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename: loadedConfig.LogsFilename,
		MaxSize:  loadedConfig.LogsMaxSize,
		Compress: true,
	}))
	log.Println("Hello world")
	log.Print("Connecting to Discord...")
	dg, err := discordgo.New("Bot " + loadedConfig.DiscordToken)
	must(err)
	dtom := make(chan string, 64)
	defer close(dtom)
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot || m.ChannelID != loadedConfig.ChannelID {
			return
		}
		log.Printf("d->m [%v] [%v]", m.Author.Username, m.Content)
		dtom <- fmt.Sprintf("[Discord] %v: %v", m.Author.Username, m.Content)
	})
	dg.Identify.Intents = discordgo.IntentsGuildMessages
	err = dg.Open()
	must(err)
	defer dg.Close()

	mtod := make(chan string, 64)
	go func() {
		for msg := range mtod {
			log.Printf("m->d [%v]", msg)
			dg.ChannelMessageSend(loadedConfig.ChannelID, msg)
		}
	}()
	log.Print("Connected to Discord.")
	log.Print("Preparing credentials...")

	client := bot.NewClient()
	credman := credentials.NewMicrosoftCredentialsManager(loadedConfig.CredentialsRoot, "88650e7e-efee-4857-b9a9-cf580a00ef43")
	botauth, err := credman.GetAuthForUsername(loadedConfig.MCUsername)
	must(err)
	if botauth == nil {
		log.Fatal("botauth nil")
	}
	client.Auth = *botauth
	basic.EventsListener{
		GameStart: func() error {
			mtod <- "Logged in"
			return nil
		},
		ChatMsg: func(c chat.Message, pos byte, uuid uuid.UUID) error {
			msg := c.Text
			for _, em := range c.Extra {
				msg += em.Text
			}
			mtod <- msg
			return nil
		},
		Disconnect: func(reason chat.Message) error {
			return DisconnectErr{Reason: reason}
		},
		HealthChange: nil,
		Death: func() error {
			mtod <- "Died lmao"
			return nil
		},
	}.Attach(client)
	must(client.JoinServer(loadedConfig.ServerAddress))
	for {
		if err = client.HandleGame(); err == nil {
			panic("HandleGame never return nil")
		}
		if err2 := new(bot.PacketHandlerError); errors.As(err, err2) {
			if err := new(DisconnectErr); errors.As(err2, err) {
				log.Print("Disconnect: ", err.Reason)
				return
			} else {
				log.Print(err2)
			}
		} else {
			log.Fatal(err)
		}
	}
}

type DisconnectErr struct {
	Reason chat.Message
}

func (d DisconnectErr) Error() string {
	return "disconnect: " + d.Reason.String()
}
