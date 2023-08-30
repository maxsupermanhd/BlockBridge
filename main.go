package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/bot/msg"
	"github.com/Tnze/go-mc/bot/playerlist"
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/net/queue"
	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
	"github.com/maxsupermanhd/WebChunk/credentials"
	"github.com/maxsupermanhd/tpsdrawer"
	"github.com/mazznoer/colorgrad"
	"github.com/natefinch/lumberjack"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

type botConf struct {
	ServerAddress     string
	DiscordToken      string
	DiscordAppID      string
	DiscordGuildID    string
	AllowedSlash      []string
	LogsFilename      string
	LogsMaxSize       int
	CredentialsRoot   string
	MCUsername        string
	ChannelID         string
	FontPath          string
	DatabaseFile      string
	AddPrefix         bool
	NameOverridesPath string
	AddTimestamps     bool
}

var (
	loadedConfig botConf
	dtom         = make(chan struct {
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
	must(json.Unmarshal(noerr(os.ReadFile(path)), &loadedConfig))
	if loadedConfig.NameOverridesPath != "" {
		must(json.Unmarshal(noerr(os.ReadFile(loadedConfig.NameOverridesPath)), &nameOverrides))
	}
	tabparams.Font = noerr(opentype.NewFace(noerr(opentype.Parse(noerr(os.ReadFile(loadedConfig.FontPath)))), &opentype.FaceOptions{
		Size:    32,
		DPI:     72,
		Hinting: font.HintingFull,
	}))
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename: loadedConfig.LogsFilename,
		MaxSize:  loadedConfig.LogsMaxSize,
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

	go func() {
		lastMessage := time.Now()
		lastAggregate := ""
		dflusher := time.NewTicker(time.Second * 5)
		for {
			select {
			case msg := <-mtod:
				msg = strings.ReplaceAll(msg, "_", "\\_")
				msg = strings.ReplaceAll(msg, "`", "\\`")
				msg = strings.ReplaceAll(msg, "*", "\\*")
				if loadedConfig.AddTimestamps {
					msg = time.Now().Format("`[02 Jan 06 15:04:05]` ") + msg
				}
				log.Printf("m-|d [%v]", msg)
				lastAggregate += msg + "\n"
				if time.Since(lastMessage).Seconds() > 2.5*float64(time.Second) {
					dg.ChannelMessageSend(loadedConfig.ChannelID, lastAggregate)
					lastAggregate = ""
					lastMessage = time.Now()
				}
			case <-dflusher.C:
				if lastAggregate == "" {
					continue
				}
				dg.ChannelMessageSend(loadedConfig.ChannelID, lastAggregate)
				lastAggregate = ""
				lastMessage = time.Now()
			}
		}
	}()
	go func() {
		for msg := range mtods {
			_, err := dg.ChannelMessageSendComplex(loadedConfig.ChannelID, &discordgo.MessageSend{
				Content: "<@343418440423309314>",
				Embed: &discordgo.MessageEmbed{
					Type:  discordgo.EmbedTypeRich,
					Title: msg,
				},
				AllowedMentions: &discordgo.MessageAllowedMentions{
					Users: []string{"343418440423309314"},
				},
			})
			if err != nil {
				log.Println(err)
			}
		}
	}()

	client := bot.NewClient()
	credman := credentials.NewMicrosoftCredentialsManager(loadedConfig.CredentialsRoot, "88650e7e-efee-4857-b9a9-cf580a00ef43")
	pla := basic.NewPlayer(client, basic.Settings{
		Locale:              "ru_RU",
		ViewDistance:        15,
		ChatMode:            0,
		DisplayedSkinParts:  basic.Jacket | basic.LeftSleeve | basic.RightSleeve | basic.LeftPantsLeg | basic.RightPantsLeg | basic.Hat,
		MainHand:            1,
		EnableTextFiltering: false,
		AllowListing:        true,
		Brand:               "Vanilla",
		ChatColors:          true,
	}, basic.EventsListener{
		GameStart: func() error {
			log.Println("Logged in")
			mtod <- "Logged in"
			return nil
		},
		Disconnect: func(reason chat.Message) error {
			log.Println("Disconnect: ", reason.String())
			mtod <- "Disconnect: " + reason.String()
			return nil
		},
		HealthChange: nil,
		Death:        nil,
	})
	msgman := msg.New(client, pla, playerlist.New(client), msg.EventsHandler{
		SystemChat: func(c chat.Message, overlay bool) error {
			if !overlay {
				mtod <- c.ClearString()
			}
			return nil
		},
		PlayerChatMessage: func(msg chat.Message, _ bool) error {
			mtod <- msg.ClearString()
			return nil
		},
		DisguisedChat: func(msg chat.Message) error {
			mtod <- msg.ClearString()
			return nil
		},
	})
	go func() {
		for m := range dtom {
			allowedsend := false
			for _, allowedid := range loadedConfig.AllowedSlash {
				if m.userid == allowedid {
					allowedsend = true
					break
				}
			}
			if !allowedsend {
				mtod <- "no chat for you"
				continue
			}
			if len(m.msg) < 1 {
				continue
			}
			if len(m.msg) > 254 {
				m.msg = m.msg[:254]
			}
			if m.msg[0] == '/' {
				allowedsend := false
				for _, allowedid := range loadedConfig.AllowedSlash {
					if m.userid == allowedid {
						allowedsend = true
						break
					}
				}
				if allowedsend {
					// log.Println([]byte(m[1:]))
					client.Conn.WritePacket(pk.Marshal(packetid.ServerboundChatCommand,
						pk.String(m.msg[1:]),            // command
						pk.Long(time.Now().UnixMilli()), // instant
						pk.Long(rand.Int63()),           // salt
						pk.VarInt(0),                    // last seen
						pk.VarInt(0),                    // msgcount?
						pk.NewFixedBitSet(20),           // ack?
					))
				}
			} else {
				msgman.SendMessage(m.msg)
			}

		}
	}()

	commandHandlers := map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"tab": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Rendering graph..."},
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
					Name:        "tps.png",
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
			tpsval, tpsn, err := GetTPSValues(db, nil)
			if err != nil {
				log.Println(err)
				mtods <- err.Error()
			}
			img := drawTPS(tpsval, tpsn)
			t := time.Duration(30 * 24 * time.Hour)
			tpsval, tpsn, err = GetTPSValues(db, &t)
			if err != nil {
				log.Println(err)
				mtods <- err.Error()
			}
			grad := noerr(colorgrad.NewGradient().
				HtmlColors("darkred", "gold", "green").
				Domain(0, 20).
				Build())
			img2 := tpsdrawer.DrawTPS(tpsval, tpsn, tpsdrawer.DrawOptions{
				DayW:       100,
				DayH:       40,
				Padding:    8,
				Spacing:    4,
				Background: color.RGBA{R: 0x36, G: 0x39, B: 0x3f, A: 0xff},
				FontColor:  color.White,
				Gradient: func(f float64) color.Color {
					if f == 0 {
						return color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xFF}
					}
					r, g, b := grad.At(f).RGB255()
					return color.RGBA{R: r, G: g, B: b, A: 0xFF}
				},
				SampleH:     32,
				Comment:     fmt.Sprint("Made by FlexCoral, tracked by Yokai0nTop, ", len(tpsval), " samples"),
				BreakMonths: true,
				BreakMonday: true,
				MeasureFunc: func(c []float64) (percentile float64) {
					percent := 1.0
					if len(c) == 0 {
						return 0
					}
					if len(c) == 1 {
						return c[0]
					}
					sort.Float64s(c)
					index := (percent / 100) * float64(len(c))
					if index == float64(int64(index)) {
						i := int(index)
						return c[i-1]
					} else if index > 1 {
						i := int(index)
						return c[i-1] + c[i]/float64(len(c))
					} else {
						return 0
					}
				},
			})
			img2w := bytes.NewBufferString("")
			err = png.Encode(img2w, img2)
			if err != nil {
				log.Println(err)
				mtods <- err.Error()
			}
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Files: []*discordgo.File{{
					Name:        "tps.png",
					ContentType: "image/png",
					Reader:      img,
				}, {
					Name:        "tps.png",
					ContentType: "image/png",
					Reader:      img2w,
				}},
			})
		},
		"lagspikes": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			options := i.ApplicationCommandData().Options
			optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
			for _, opt := range options {
				optionMap[opt.Name] = opt
			}
			var dur *time.Duration
			if durRaw, ok := optionMap["duration"]; ok {
				durs, err := time.ParseDuration(durRaw.StringValue())
				if err != nil {
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{Content: "Invalid duration: " + err.Error()},
					})
					return
				}
				dur = &durs
			}
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: "Getting your data..."},
			})
			var r0, r1 io.Reader
			spikes, err := GetLastLagspikes(db, dur)
			if err != nil {
				errstr := err.Error()
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &errstr,
				})
				return
			}
			r0 = FormatLagspikes(spikes)
			r1, err = GetRankedLagspikes(spikes)
			if err != nil {
				errstr := err.Error()
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &errstr,
				})
				return
			}
			msg := "Last 150 lagspikes"
			if dur != nil {
				msg = fmt.Sprintf("Lagspike data for past %s", dur.String())
			}
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &msg,
				Files: []*discordgo.File{{
					Name:        "lastLagspikes.txt",
					ContentType: "text/plain",
					Reader:      r0,
				}, {
					Name:        "lagspikeRank.txt",
					ContentType: "text/plain",
					Reader:      r1,
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
	prevTPS := float32(-1)
	client.Events.AddListener(bot.PacketHandler{
		ID:       packetid.ClientboundSetTime,
		Priority: 20,
		F: func(p pk.Packet) error {
			var (
				worldAge  pk.Long
				timeOfDay pk.Long
			)
			must(p.Scan(&worldAge, &timeOfDay))
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
			_, err := db.Exec(`insert into tps (whenlogged, tpsvalue) values (unixepoch(), $1)`, since)
			if err != nil {
				log.Println("Error inserting tps value ", err)
			}
			if prevTPS-since > 10 {
				resp := make(chan interface{})
				tabactions <- tabaction{
					op:   "snapshot",
					resp: resp,
				}
				pl := (<-resp).(string)
				_, err := db.Exec(`insert into lagspikes (whenlogged, tpsprev, tpscurrent, players) values (unixepoch(), $1, $2, $3)`, prevTPS, since, pl)
				if err != nil {
					log.Println("Error inserting lagspikes value ", err)
				}
			}
			prevTPS = since
			client.Conn.WritePacket(pk.Marshal(packetid.ServerboundSwing, pk.VarInt(0)))
			return nil
		},
	})
	client.Events.AddListener(
		bot.PacketHandler{
			Priority: 64, ID: packetid.ClientboundPlayerInfoUpdate,
			F: handlePlayerInfoUpdatePacket,
		},
		bot.PacketHandler{
			Priority: 64, ID: packetid.ClientboundPlayerInfoRemove,
			F: handlePlayerInfoRemovePacket,
		},
		bot.PacketHandler{
			Priority: 20, ID: packetid.ClientboundTabList,
			F: handleTabHeaderFooter,
		})
	for {
		timeout := time.Second * 20
		tabactions <- tabaction{op: "clear"}
		log.Println("Getting auth...")
		botauth, err := credman.GetAuthForUsername(loadedConfig.MCUsername)
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
		log.Println("Connecting to", loadedConfig.ServerAddress)
		dialctx, dialctxcancel := context.WithTimeout(context.Background(), timeout)
		dialer := net.Dialer{Timeout: timeout, Deadline: time.Now().Add(timeout), KeepAlive: 1 * time.Second}
		mcdialer := (*mcnet.Dialer)(&dialer)
		err = client.JoinServerWithOptions(loadedConfig.ServerAddress, bot.JoinOptions{
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
