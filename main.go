package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/bot/msg"
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/bwmarrin/discordgo"
	"github.com/maxsupermanhd/WebChunk/credentials"
	"github.com/natefinch/lumberjack"
)

type botConf struct {
	ServerAddress   string
	DiscordToken    string
	DiscordAppID    string
	DiscordGuildID  string
	AllowedSlash    []string
	LogsFilename    string
	LogsMaxSize     int
	CredentialsRoot string
	MCUsername      string
	ChannelID       string
	FontPath        string
}

var loadedConfig botConf

func must(err error) {
	if err != nil {
		debug.PrintStack()
		log.Fatal(err)
	}
}

func noerr[T any](ret T, err error) T {
	must(err)
	return ret
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

type TabPlayer struct {
	name string
	ping int
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	loadConfig()
	log.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename: loadedConfig.LogsFilename,
		MaxSize:  loadedConfig.LogsMaxSize,
		Compress: true,
	}))
	tabplayers := map[pk.UUID]TabPlayer{}
	tabtop := new(chat.Message)
	tabbottom := new(chat.Message)
	log.Println("Hello world")
	log.Print("Connecting to Discord...")
	dg, err := discordgo.New("Bot " + loadedConfig.DiscordToken)
	must(err)
	dg.Identify.Intents |= discordgo.IntentMessageContent
	dtom := make(chan string, 64)
	defer close(dtom)
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot || m.ChannelID != loadedConfig.ChannelID {
			return
		}
		log.Printf("d->m [%v] [%v]", m.Author.Username, m.Content)
		dtom <- fmt.Sprintf("[Discord] %v#%v: %v", m.Author.Username, m.Author.Discriminator, m.Content)
	})
	noerr(dg.ApplicationCommandCreate(loadedConfig.DiscordAppID, loadedConfig.DiscordGuildID, &discordgo.ApplicationCommand{
		ID:                       "tabCommand",
		ApplicationID:            loadedConfig.DiscordAppID,
		GuildID:                  loadedConfig.DiscordGuildID,
		Version:                  "1",
		Type:                     discordgo.ChatApplicationCommand,
		Name:                     "tab",
		NameLocalizations:        &map[discordgo.Locale]string{},
		DefaultPermission:        new(bool),
		DefaultMemberPermissions: new(int64),
		DMPermission:             new(bool),
		Description:              "renders out tab",
		DescriptionLocalizations: &map[discordgo.Locale]string{},
		Options:                  []*discordgo.ApplicationCommandOption{},
	}))
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
	pla := basic.NewPlayer(client, basic.Settings{
		Locale:              "ru_RU",
		ViewDistance:        15,
		ChatMode:            0,
		DisplayedSkinParts:  basic.Jacket | basic.LeftSleeve | basic.RightSleeve | basic.LeftPantsLeg | basic.RightPantsLeg | basic.Hat,
		MainHand:            1,
		EnableTextFiltering: false,
		AllowListing:        false,
		Brand:               "Pepe's chatbot",
	}, basic.EventsListener{
		GameStart: func() error {
			mtod <- "Logged in"
			return nil
		},
		SystemMsg: func(c chat.Message, overlay bool) error {
			if !overlay {
				mtod <- c.ClearString()
			}
			return nil
		},
		Disconnect: func(reason chat.Message) error {
			log.Println("Disconnect: ", reason.String())
			return nil
		},
		HealthChange: nil,
		Death:        nil,
	})
	msgman := msg.New(client, pla, msg.EventsHandler{
		PlayerChatMessage: func(msg chat.Message) error {
			mtod <- msg.ClearString()
			return nil
		},
	})
	go func() {
		for msg := range dtom {
			if len(msg) > 254 {
				msg = msg[:254]
			}
			msgman.SendMessage(msg)
		}
	}()
	commandHandlers := map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"tab": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			// msg := "People online: "
			// for _, v := range tabplayers {
			// 	msg += fmt.Sprintf("%s (%d) ", v.name, v.ping)
			// }
			img := drawTab(tabplayers, tabtop, tabbottom)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					TTS:             false,
					Content:         "",
					Components:      []discordgo.MessageComponent{},
					Embeds:          []*discordgo.MessageEmbed{},
					AllowedMentions: &discordgo.MessageAllowedMentions{},
					Files: []*discordgo.File{{
						Name:        "tab.png",
						ContentType: "image/png",
						Reader:      img,
					}},
					Flags:    0,
					Choices:  []*discordgo.ApplicationCommandOptionChoice{},
					CustomID: "",
					Title:    "",
				},
			})
		},
	}
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
	client.Events.AddListener(bot.PacketHandler{
		ID:       packetid.ClientboundTabList,
		Priority: 20,
		F: func(p pk.Packet) error {
			must(p.Scan(tabtop, tabbottom))
			// log.Println(string(noerr(tabtop.MarshalJSON())))
			return nil
		},
	})
	client.Events.AddListener(bot.PacketHandler{
		ID:       packetid.ClientboundPlayerInfo,
		Priority: 20,
		F: func(p pk.Packet) error {
			var action pk.VarInt
			must(p.Scan(&action))
			switch action {
			case 0:
				arr := []PlayerInfoAdd{}
				must(p.Scan(&action, pk.Ary[pk.VarInt]{Ary: &arr}))
				for _, v := range arr {
					tabplayers[v.uuid] = TabPlayer{
						name: string(v.name),
						ping: int(v.ping),
					}
					// log.Printf("%#v: {\"%s\", %d},\n", v.uuid, v.name, v.ping)
				}
			case 1:
				// arr := []PlayerInfoUpdateGamemode{}
			case 2:
				arr := []PlayerInfoUpdatePing{}
				must(p.Scan(&action, pk.Ary[pk.VarInt]{Ary: &arr}))
				for _, v := range arr {
					t, ok := tabplayers[v.uuid]
					if ok {
						t.ping = int(v.ping)
						tabplayers[v.uuid] = t
					}
				}
			case 3:
			case 4:
				arr := []pk.UUID{}
				must(p.Scan(&action, pk.Ary[pk.VarInt]{Ary: &arr}))
				for _, v := range arr {
					_, ok := tabplayers[v]
					if ok {
						delete(tabplayers, v)
					}
				}
			}
			return nil
		},
	})
	must(client.JoinServerWithOptions(loadedConfig.ServerAddress, bot.JoinOptions{
		Dialer:      nil,
		Context:     nil,
		NoPublicKey: true,
		KeyPair:     nil,
	}))
	must(client.HandleGame())
	// sigchan := make(chan os.Signal, 1)
	// signal.Notify(sigchan, os.Interrupt)
	// <-sigchan
}

type DisconnectErr struct {
	Reason chat.Message
}

func (d DisconnectErr) Error() string {
	return "disconnect: " + d.Reason.String()
}

// func fixchat(m chat.Message) string {
// 	var msg strings.Builder
// 	text, _ := chat.TransCtrlSeq(m.Text, false)
// 	msg.WriteString(text)

// 	//handle translate
// 	if m.Translate != "" {
// 		args := make([]interface{}, len(m.With))
// 		for i, v := range m.With {
// 			var arg chat.Message
// 			_ = arg.UnmarshalJSON(v) //ignore error
// 			args[i] = arg.ClearString()
// 		}
// 		tr, ok := translations.Map[m.Translate]
// 		if !ok {
// 			_, _ = fmt.Fprintf(&msg, m.Translate, args...)
// 		} else {
// 			_, _ = fmt.Fprintf(&msg, tr, args...)
// 		}
// 	}

// 	if m.Extra != nil {
// 		for i := range m.Extra {
// 			msg.WriteString(m.Extra[i].ClearString())
// 		}
// 	}
// 	return msg.String()
// }
