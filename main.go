package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tnze/go-mc/chat"
	"github.com/bwmarrin/discordgo"
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
	credentials.NewMicrosoftCredentialsManager(loadedConfig.CredentialsRoot, "88650e7e-efee-4857-b9a9-cf580a00ef43")
	log.Print("Connecting to Discord...")
	dg, err := discordgo.New("Bot " + loadedConfig.DiscordToken)
	if err != nil {
		log.Println("error creating Discord session,", err)
		return
	}
	dtom := make(chan string, 64)
	defer close(dtom)
	// mtod := make(chan string, 64)
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot || m.ChannelID != loadedConfig.ChannelID {
			return
		}
		log.Printf("d->m [%v] [%v]", m.Author.Username, m.Content)
		dtom <- fmt.Sprintf("[Discord] %v: %v", m.Author.Username, m.Content)
	})
	dg.Identify.Intents = discordgo.IntentsGuildMessages
	err = dg.Open()
	if err != nil {
		log.Println("error opening connection,", err)
		return
	}
	defer dg.Close()
	log.Print("Connected to Discord.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	log.Println("Roger, stopping shit.")
}
