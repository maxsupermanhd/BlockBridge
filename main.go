package main

import (
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/Tnze/go-mc/chat"
	"github.com/natefinch/lumberjack"
)

type botConf struct {
	ServerAddress string
	DiscordToken  string
	AllowedSlash  []string
	SpamInterval  int
	SpamMessage   chat.Message
	LogsFilename  string
	LogsMaxSize   int
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
}
