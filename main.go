package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/bot/msg"
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/maxsupermanhd/WebChunk/credentials"
	"github.com/maxsupermanhd/tabdrawer"
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
}

var loadedConfig botConf

var (
	nameOverrides = map[string]chat.Message{}
)

func loadConfig() {
	path := os.Getenv("BLOCKBRIDGE_CONFIG")
	if path == "" {
		path = "config.json"
	}
	must(json.Unmarshal(noerr(os.ReadFile(path)), &loadedConfig))
	if loadedConfig.NameOverridesPath != "" {
		must(json.Unmarshal(noerr(os.ReadFile(loadedConfig.NameOverridesPath)), &nameOverrides))
	}
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	loadConfig()
	log.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename: loadedConfig.LogsFilename,
		MaxSize:  loadedConfig.LogsMaxSize,
		Compress: true,
	}))
	tabplayers := map[uuid.UUID]tabdrawer.TabPlayer{}
	tabtop := new(chat.Message)
	tabbottom := new(chat.Message)
	log.Println("Hello world")

	db := noerr(sql.Open("sqlite3", loadedConfig.DatabaseFile))
	db.Exec(`create table if not exists tps (whenlogged timestamp, tpsvalue float);`)

	log.Print("Connecting to Discord...")
	dg, err := discordgo.New("Bot " + loadedConfig.DiscordToken)
	must(err)
	dg.Identify.Intents |= discordgo.IntentMessageContent
	dtom := make(chan struct {
		msg    string
		userid string
	}, 64)
	defer close(dtom)
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot || m.ChannelID != loadedConfig.ChannelID {
			return
		}
		log.Printf("d->m [%v] [%v]", m.Author.Username, m.Content)
		if loadedConfig.AddPrefix {
			dtom <- struct {
				msg    string
				userid string
			}{
				msg:    fmt.Sprintf("[Discord] %v#%v: %v", m.Author.Username, m.Author.Discriminator, m.Content),
				userid: m.Author.ID,
			}
		} else {
			dtom <- struct {
				msg    string
				userid string
			}{
				msg:    m.Content,
				userid: m.Author.ID,
			}
		}
	})
	noerr(dg.ApplicationCommandCreate(loadedConfig.DiscordAppID, loadedConfig.DiscordGuildID, &discordgo.ApplicationCommand{
		ID:            "tabCommand",
		ApplicationID: loadedConfig.DiscordAppID,
		GuildID:       loadedConfig.DiscordGuildID,
		Version:       "1",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "tab",
		Description:   "renders out tab",
	}))
	noerr(dg.ApplicationCommandCreate(loadedConfig.DiscordAppID, loadedConfig.DiscordGuildID, &discordgo.ApplicationCommand{
		ID:            "tpsCommand",
		ApplicationID: loadedConfig.DiscordAppID,
		GuildID:       loadedConfig.DiscordGuildID,
		Version:       "1",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "tps",
		Description:   "renders out tps chart",
	}))
	noerr(dg.ApplicationCommandCreate(loadedConfig.DiscordAppID, loadedConfig.DiscordGuildID, &discordgo.ApplicationCommand{
		ID:            "reloadConfigCommand",
		ApplicationID: loadedConfig.DiscordAppID,
		GuildID:       loadedConfig.DiscordGuildID,
		Version:       "1",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "reloadconfig",
		Description:   "reloads config",
	}))
	dg.Identify.Intents = discordgo.IntentsGuildMessages
	err = dg.Open()
	must(err)
	defer dg.Close()

	mtod := make(chan string, 64)
	mtods := make(chan discordgo.MessageSend, 64)
	go func() {
		for msg := range mtod {
			log.Printf("m->d [%v]", msg)
			dg.ChannelMessageSend(loadedConfig.ChannelID, msg)
		}
	}()
	go func() {
		for msg := range mtods {
			dg.ChannelMessageSendComplex(loadedConfig.ChannelID, &msg)
		}
	}()
	log.Print("Connected to Discord.")

	client := bot.NewClient()
	credman := credentials.NewMicrosoftCredentialsManager(loadedConfig.CredentialsRoot, "88650e7e-efee-4857-b9a9-cf580a00ef43")
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
		for m := range dtom {
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
						pk.VarInt(0),                    // empty collection
						pk.Boolean(false),               // signed preview
						pk.VarInt(0),                    // last seen collection
						pk.Boolean(false),               // no last received
					))
				}
			} else {
				msgman.SendMessage(m.msg)
			}

		}
	}()

	log.Println("Loading tab params...")
	tabparams := tabdrawer.TabParameters{
		Font: noerr(opentype.NewFace(noerr(opentype.Parse(noerr(os.ReadFile(loadedConfig.FontPath)))), &opentype.FaceOptions{
			Size:    32,
			DPI:     72,
			Hinting: font.HintingFull,
		})),
		ColumnSpacing:       8,
		RowSpacing:          1,
		RowAdditionalHeight: 2,
		OverridePlayerName: func(u uuid.UUID) *chat.Message {
			v, ok := nameOverrides[u.String()]
			if ok {
				return &v
			}
			v, ok = nameOverrides[tabplayers[u].Name.ClearString()]
			if ok {
				return &v
			}
			return nil
		},
	}

	commandHandlers := map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"reloadconfig": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			loadConfig()
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Reloaded.",
				},
			})
		},
		"tab": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			img := tabdrawer.DrawTab(tabplayers, tabtop, tabbottom, &tabparams)
			buff := bytes.NewBufferString("")
			must(png.Encode(buff, img))
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Files: []*discordgo.File{{
						Name:        "tab.png",
						ContentType: "image/png",
						Reader:      buff,
					}},
				},
			})
		},
		"tps": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Rendering graph... Hold tight!",
				},
			})
			grresp := make(chan io.Reader, 1)
			go func() {
				rows := noerr(db.Query(`select cast(whenlogged as int), tpsvalue from tps where whenlogged + 24*60*60 > unixepoch() order by whenlogged asc;`))
				defer rows.Close()
				tpsval := []time.Time{}
				tpsn := []float64{}
				for rows.Next() {
					var (
						when int64
						tps  float64
					)
					must(rows.Scan(&when, &tps))
					tpsunix := time.Unix(when, 0)
					tpsavgs := float64(0)
					tpsavgc := float64(0)
					timeavg := 20
					ticksavg := timeavg * 20
					for i := len(tpsn); i > 0 && i+ticksavg < len(tpsn); i++ {
						if tpsunix.Sub(tpsval[i]) > time.Duration(timeavg)*time.Second {
							break
						}
						tpsavgc++
						tpsavgs += tpsn[i]
					}
					tpsval = append(tpsval, tpsunix)
					if tpsavgc > 0 {
						tpsn = append(tpsn, tpsavgs/tpsavgc)
					} else {
						tpsn = append(tpsn, tps)
					}
				}
				// tpsm := map[time.Time]float64{}
				// for rows.Next() {
				// 	var (
				// 		when int64
				// 		tps  float64
				// 	)
				// 	must(rows.Scan(&when, &tps))
				// 	tpsm[time.Unix(when, 0)] = tps
				// }
				grresp <- drawTPS(tpsval, tpsn)
			}()
			img := <-grresp
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Files: []*discordgo.File{{
					Name:        "tps.png",
					ContentType: "image/png",
					Reader:      img,
				}},
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
			// log.Println(string(noerr(tabbottom.MarshalJSON())))
			return nil
		},
	})
	lastTimeUpdate := time.Now()
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
			noerr(db.Exec(`insert into tps (whenlogged, tpsvalue) values (unixepoch(), $1)`, since))
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
					// spew.Dump(v.props)
					var headimg image.Image
					headimg = nil
					for _, vv := range v.props {
						if vv.name == "textures" {
							var tex map[string]interface{}
							must(json.Unmarshal(noerr(base64.StdEncoding.DecodeString(string(vv.value))), &tex))
							texx, ok := tex["textures"].(map[string]interface{})
							if !ok {
								continue
							}
							texxx, ok := texx["SKIN"].(map[string]interface{})
							if !ok {
								continue
							}
							texurl, ok := texxx["url"].(string)
							if !ok {
								continue
							}
							log.Println("GET ", texurl)
							textureresp, err := http.Get(texurl)
							if err != nil {
								log.Println("Error fetching ", texurl, err)
								continue
							}
							teximg, err := png.Decode(textureresp.Body)
							if err != nil {
								log.Println("Error decoding ", texurl, err)
								continue
							}
							headimg, _ = CropImage(teximg, image.Rect(8, 8, 16, 16))
						}
					}
					tabplayers[uuid.UUID(v.uuid)] = tabdrawer.TabPlayer{
						Name:        chat.Message{Text: string(v.name)},
						Ping:        int(v.ping),
						HeadTexture: headimg,
					}
					log.Printf("Player join %v %v", uuidToString(v.uuid), v.name)
				}
			case 1:
				arr := []PlayerInfoUpdateGamemode{}
				must(p.Scan(&action, pk.Ary[pk.VarInt]{Ary: &arr}))
				gmodes := []string{"Survival", "Creative", "Adventure", "Spectator"}
				for _, v := range arr {
					if v.gamemode < 0 || v.gamemode > 3 {
						log.Printf("Overflow of the gamemode update: %#v", v)
						continue
					}
					if v.gamemode != 1 {
						log.Printf("Gamemode change %v %v", uuidToString(v.uuid), v.gamemode)
						mtods <- discordgo.MessageSend{
							Content:    fmt.Sprintf("Player %v changed game mode to %v", uuidToString(v.uuid), gmodes[v.gamemode]),
							Embeds:     []*discordgo.MessageEmbed{},
							TTS:        false,
							Components: []discordgo.MessageComponent{},
							Files:      []*discordgo.File{},
							AllowedMentions: &discordgo.MessageAllowedMentions{
								Parse:       []discordgo.AllowedMentionType{},
								Roles:       []string{},
								Users:       []string{"343418440423309314"},
								RepliedUser: false,
							},
							Reference: &discordgo.MessageReference{},
							File:      &discordgo.File{},
							Embed:     &discordgo.MessageEmbed{},
						}
					}
				}
			case 2:
				arr := []PlayerInfoUpdatePing{}
				must(p.Scan(&action, pk.Ary[pk.VarInt]{Ary: &arr}))
				for _, v := range arr {
					t, ok := tabplayers[uuid.UUID(v.uuid)]
					if ok {
						t.Ping = int(v.ping)
						tabplayers[uuid.UUID(v.uuid)] = t
						// log.Printf("Ping update for the player %v %5d %v", uuidToString(v.uuid), v.ping, t.Name)
					} else {
						log.Printf("Ping update of the player that is not in the tab!!! %#v", v)
					}
				}
			case 3:
				arr := []PlayerInfoUpdateDisplayName{}
				must(p.Scan(&action, pk.Ary[pk.VarInt]{Ary: &arr}))
				for _, v := range arr {
					t, ok := tabplayers[uuid.UUID(v.uuid)]
					if ok {
						t.Name = *v.displayname
					}
				}
			case 4:
				arr := []pk.UUID{}
				must(p.Scan(&action, pk.Ary[pk.VarInt]{Ary: &arr}))
				for _, v := range arr {
					tt, ok := tabplayers[uuid.UUID(v)]
					if ok {
						log.Printf("Player leave %v %v", uuidToString(v), tt.Name)
						delete(tabplayers, uuid.UUID(v))
					} else {
						log.Printf("Delete of the player that is not in the tab!!! %#v", uuidToString(v))
					}
				}
			}
			return nil
		},
	})
	for {
		for k := range tabplayers {
			delete(tabplayers, k)
		}
		log.Println("Getting auth...")
		botauth, err := credman.GetAuthForUsername(loadedConfig.MCUsername)
		must(err)
		if botauth == nil {
			log.Fatal("botauth nil")
		}
		client.Auth = *botauth
		log.Println("Connecting to", loadedConfig.ServerAddress)
		err = client.JoinServerWithOptions(loadedConfig.ServerAddress, bot.JoinOptions{
			Dialer:      &net.Dialer{Timeout: 10 * time.Second},
			Context:     nil,
			NoPublicKey: true,
			KeyPair:     nil,
		})
		if err != nil {
			mtod <- "Failed to join server: " + err.Error()
			time.Sleep(10 * time.Second)
			continue
		}
		err = client.HandleGame()
		if err != nil {
			mtod <- "Disconnected: " + err.Error()
			time.Sleep(10 * time.Second)
		}
	}
	// sigchan := make(chan os.Signal, 1)
	// signal.Notify(sigchan, os.Interrupt)
	// <-sigchan
}

func CropImage(img image.Image, cropRect image.Rectangle) (cropImg image.Image, newImg bool) {
	//Interface for asserting whether `img`
	//implements SubImage or not.
	//This can be defined globally.
	type CropableImage interface {
		image.Image
		SubImage(r image.Rectangle) image.Image
	}

	if p, ok := img.(CropableImage); ok {
		// Call SubImage. This should be fast,
		// since SubImage (usually) shares underlying pixel.
		cropImg = p.SubImage(cropRect)
	} else if cropRect = cropRect.Intersect(img.Bounds()); !cropRect.Empty() {
		// If `img` does not implement `SubImage`,
		// copy (and silently convert) the image portion to RGBA image.
		rgbaImg := image.NewRGBA(cropRect)
		for y := cropRect.Min.Y; y < cropRect.Max.Y; y++ {
			for x := cropRect.Min.X; x < cropRect.Max.X; x++ {
				rgbaImg.Set(x, y, img.At(x, y))
			}
		}
		cropImg = rgbaImg
		newImg = true
	} else {
		// Return an empty RGBA image
		cropImg = &image.RGBA{}
		newImg = true
	}

	return cropImg, newImg
}

func uuidToString(uuid [16]byte) string {
	var buf [36]byte
	encodeUUIDToHex(buf[:], uuid)
	return string(buf[:])
}

func encodeUUIDToHex(dst []byte, uuid [16]byte) {
	hex.Encode(dst[:], uuid[:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], uuid[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], uuid[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], uuid[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:], uuid[10:])
}
