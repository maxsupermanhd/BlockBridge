package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/bot/msg"
	"github.com/Tnze/go-mc/bot/playerlist"
	"github.com/Tnze/go-mc/data/packetid"
	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/net/queue"
	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
	"github.com/maxsupermanhd/WebChunk/credentials"
	"github.com/maxsupermanhd/lac"
	"github.com/natefinch/lumberjack"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// type botConf struct {
// 	ServerAddress                string
// 	DiscordToken                 string
// 	DiscordAppID                 string
// 	DiscordGuildID               string
// 	AllowedSlash                 []string
// 	LogsFilename                 string
// 	LogsMaxSize                  int
// 	CredentialsRoot              string
// 	MCUsername                   string
// 	ChannelID                    string
// 	FontPath                     string
// 	DatabaseFile                 string
// 	AddPrefix                    bool
// 	NameOverridesPath            string
// 	AddTimestamps                bool
// 	CaptureLagspikes             bool
// 	StatusChannelID              string
// 	StatusRefreshIntervalSeconds int
// }

var (
	cfg  *lac.Conf
	dtom = make(chan struct {
		msg    string
		userid string
	}, 128)
	mtod   = make(chan string, 128)
	mtods  = make(chan string, 64)
	gmodes = []string{"Survival", "Creative", "Adventure", "Spectator"}
)

func init() {
	path := os.Getenv("BLOCKBRIDGE_CONFIG")
	if path == "" {
		path = "config.json"
	}
	cfg = noerr(lac.FromFileJSON(path))
	nameOverridesPath, ok := cfg.GetString("NameOverridesPath")
	if ok {
		must(json.Unmarshal(noerr(os.ReadFile(nameOverridesPath)), &nameOverrides))
	}
	tabparams.Font = noerr(opentype.NewFace(noerr(opentype.Parse(noerr(os.ReadFile(cfg.GetDString("Minecraft-Regular.otf", "FontPath"))))), &opentype.FaceOptions{
		Size:    32,
		DPI:     72,
		Hinting: font.HintingFull,
	}))
	cachedStatusMessageID = cfg.GetDSString("", "CachedStatusMessageID")
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename: cfg.GetDString("logs/chatlog.log", "LogsFilename"),
		MaxSize:  cfg.GetDInt(10, "LogsMaxSize"),
		Compress: true,
	}))
	log.Println("Hello world")

	defer close(dtom)
	defer close(mtod)

	go TabProcessor()
	defer close(tabactions)

	db := SetupDatabase()
	defer db.Close()

	dg := OpenDiscord()
	defer dg.Close()

	go statusUpdater(dg, db)

	go pipeMessagesToDiscord(dg)
	go pipeImportantMessagesToDiscord(dg)

	client := bot.NewClient()
	credman := credentials.NewMicrosoftCredentialsManager(cfg.GetDString("cmd/auth/", "CredentialsRoot"), "88650e7e-efee-4857-b9a9-cf580a00ef43")
	pla := basic.NewPlayer(client, botBasicSettings, botBasicEvents)
	msgman := msg.New(client, pla, playerlist.New(client), botMessageEvents)
	go pipeMessagesFromDiscord(client, msgman)

	commandHandlers := map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"tab": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Rendering tab..."},
			})
			rsp := make(chan interface{})
			tabactions <- tabaction{
				op:   "draw",
				resp: rsp,
			}
			buff := bytes.NewBufferString("")
			must(png.Encode(buff, (<-rsp).(image.Image)))
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Files: []*discordgo.File{{
					Name:        "tab.png",
					ContentType: "image/png",
					Reader:      buff,
				}},
			})
		},
		"lasttpssamples": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Getting your data..."},
			})
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Files: []*discordgo.File{{
					Name:        "tpslog.txt",
					ContentType: "text/plain",
					Reader:      GetLastTPSValues(db),
				}},
			})
		},
		"tps": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Rendering graph..."},
			})
			tpschart, tpsheat, profiler, err := getStatusTPS(db)
			if err != nil {
				cnt := err.Error()
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &cnt,
				})
				return
			}
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &profiler,
				Files: []*discordgo.File{{
					Name:        "tpsChart.png",
					ContentType: "image/png",
					Reader:      tpschart,
				}, {
					Name:        "tpsHeat.png",
					ContentType: "image/png",
					Reader:      tpsheat,
				}},
			})
		},
	}
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
	lastTimeUpdate := time.Now()
	// prevTPS := float32(-1)
	client.Events.AddListener(bot.PacketHandler{
		ID:       packetid.ClientboundSetTime,
		Priority: 20,
		F: func(p pk.Packet) error {
			var (
				worldAge  pk.Long
				timeOfDay pk.Long
			)
			err := p.Scan(&worldAge, &timeOfDay)
			if err != nil {
				log.Printf("Failed to scan world age and time of day: %s", err.Error())
				return nil
			}
			last := float32(time.Since(lastTimeUpdate).Milliseconds()) / float32(1000)
			if last == 0 {
				last = 1
			}
			since := float32(20) / last
			if since < 0 {
				since = 0
			}
			if since > 20 {
				since = 20
			}
			lastTimeUpdate = time.Now()
			go func(when time.Time, since float32) {
				tabresp := make(chan interface{})
				tabactions <- tabaction{
					op:   "count",
					resp: tabresp,
				}
				tablen := (<-tabresp).(int)
				_, err = db.Exec(context.Background(), `insert into tps (whenlogged, tpsvalue, playercount) values ($1, $2, $3)`, when, since, tablen)
				if err != nil {
					log.Printf("Error inserting tps value: %s", err.Error())
				}
			}(time.Now(), since)
			// prevTPS = since
			client.Conn.WritePacket(pk.Marshal(packetid.ServerboundSwing, pk.VarInt(0)))
			return nil
		},
	})
	addTabHandlers(client)
	for {
		tabactions <- tabaction{op: "clear"}
		timeout := time.Second * 60
		log.Println("Getting auth...")
		botauth, err := credman.GetAuthForUsername(cfg.GetDString("Steve", "MCUsername"))
		if err != nil {
			mtod <- "Failed to acquire auth: " + err.Error()
			time.Sleep(timeout)
			continue
		}
		if botauth == nil {
			mtod <- "Bot auth is nil!"
			time.Sleep(timeout)
			continue
		}
		client.Auth = *botauth
		log.Println("Connecting to", cfg.GetDString("localhost", "ServerAddress"))
		dialctx, dialctxcancel := context.WithTimeout(context.Background(), timeout)
		dialer := net.Dialer{Timeout: timeout, Deadline: time.Now().Add(timeout), KeepAlive: 1 * time.Second}
		mcdialer := (*mcnet.Dialer)(&dialer)
		err = client.JoinServerWithOptions(cfg.GetDString("localhost", "ServerAddress"), bot.JoinOptions{
			MCDialer:    mcdialer,
			Context:     dialctx,
			NoPublicKey: true,
			KeyPair:     nil,
			QueueRead:   queue.NewLinkedQueue[pk.Packet](),
			QueueWrite:  queue.NewLinkedQueue[pk.Packet](),
		})
		dialctxcancel()
		if err != nil {
			mtod <- "Failed to join server: " + err.Error()
			// cancelDisconnectTimer <- true
			time.Sleep(timeout)
			continue
		}
		log.Println("Connected, starting HandleGame")
		err = client.HandleGame()
		log.Println("HandleGame exited")
		// cancelDisconnectTimer <- true
		client.Close()
		if err != nil {
			mtod <- "Disconnected: " + err.Error()
			time.Sleep(timeout)
		}
	}
}
