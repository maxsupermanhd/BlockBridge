package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v4/pgxpool"
	_ "github.com/mattn/go-sqlite3"
	"github.com/maxsupermanhd/WebChunk/credentials"
	"github.com/maxsupermanhd/go-vmc/v762/bot"
	"github.com/maxsupermanhd/go-vmc/v762/bot/basic"
	"github.com/maxsupermanhd/go-vmc/v762/bot/msg"
	"github.com/maxsupermanhd/go-vmc/v762/bot/playerlist"
	"github.com/maxsupermanhd/go-vmc/v762/data/packetid"
	mcnet "github.com/maxsupermanhd/go-vmc/v762/net"
	pk "github.com/maxsupermanhd/go-vmc/v762/net/packet"
	"github.com/maxsupermanhd/go-vmc/v762/net/queue"
	"github.com/maxsupermanhd/lac"
	"github.com/natefinch/lumberjack"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

var (
	cfg  *lac.Conf
	db   *pgxpool.Pool
	dtom = make(chan struct {
		msg    string
		userid string
	}, 128)
	mtod   = make(chan string, 128)
	mtods  = make(chan string, 64)
	gmodes = []string{"Survival", "Creative", "Adventure", "Spectator"}
)

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	path := os.Getenv("BLOCKBRIDGE_CONFIG")
	if path == "" {
		path = "config.json"
	}
	cfg = noerr(lac.FromFileJSON(path))
	log.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename: cfg.GetDString("logs/chatlog.log", "LogsFilename"),
		MaxSize:  cfg.GetDInt(10, "LogsMaxSize"),
		Compress: true,
	}))
	log.Println("Hello world")
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
	db = SetupDatabase()
}

func main() {
	defer close(dtom)
	defer close(mtod)

	go TabProcessor()
	defer close(tabactions)

	defer db.Close()

	dg := OpenDiscord()
	defer dg.Close()

	go statusUpdater(dg, db)

	go pipeMessagesToDiscord(dg)
	go pipeImportantMessagesToDiscord(dg)

	go pingbackonlineDelivery(dg)

	client := bot.NewClient()
	credman := credentials.NewMicrosoftCredentialsManager(cfg.GetDString("cmd/auth/", "CredentialsRoot"), "88650e7e-efee-4857-b9a9-cf580a00ef43")
	pla := basic.NewPlayer(client, botBasicSettings, botBasicEvents)
	msgman := msg.New(client, pla, playerlist.New(client), botMessageEvents)
	go pipeMessagesFromDiscord(client, msgman)

	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		case discordgo.InteractionMessageComponent:
			if h, ok := componentHandlers[i.MessageComponentData().CustomID]; ok {
				h(s, i)
			}
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
		client.Auth = bot.Auth{
			Name: botauth.Name,
			UUID: botauth.UUID,
			AsTk: botauth.AsTk,
		}
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
		firePingbackonlineEvent(pingbackonlineEventTypeConnected)
		err = client.HandleGame()
		log.Println("HandleGame exited")
		firePingbackonlineEvent(pingbackonlineEventTypeDisonnected)
		// cancelDisconnectTimer <- true

		client.Close()
		if err != nil {
			mtod <- "Disconnected: " + err.Error()
			time.Sleep(timeout)
		}
	}
}
