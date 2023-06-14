package main

import (
	"bytes"
	"context"
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
	"sync"
	"time"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/bot/msg"
	"github.com/Tnze/go-mc/bot/playerlist"
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/chat/sign"
	"github.com/Tnze/go-mc/data/packetid"
	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil/user"
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
	tabplayersmutex := sync.Mutex{}
	tabtop := new(chat.Message)
	tabbottom := new(chat.Message)
	log.Println("Hello world")

	db := noerr(sql.Open("sqlite3", loadedConfig.DatabaseFile))
	noerr(db.Exec(`create table if not exists tps (whenlogged timestamp, tpsvalue float);`))
	noerr(db.Exec(`create index if not exists tps_index on tps (whenlogged);`))

	log.Print("Connecting to Discord...")
	dg, err := discordgo.New("Bot " + loadedConfig.DiscordToken)
	must(err)
	dg.Identify.Intents |= discordgo.IntentMessageContent
	dtom := make(chan struct {
		msg    string
		userid string
	}, 64)
	defer close(dtom)
	dg.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
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
	noerr(dg.ApplicationCommandCreate(loadedConfig.DiscordAppID, loadedConfig.DiscordGuildID, &discordgo.ApplicationCommand{
		ID:            "lasttpssamples",
		ApplicationID: loadedConfig.DiscordAppID,
		GuildID:       loadedConfig.DiscordGuildID,
		Version:       "1",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "lasttpssamples",
		Description:   "spews out last tps sample",
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
		AllowListing:        true,
		Brand:               "Vanilla",
		ChatColors:          true,
	}, basic.EventsListener{
		GameStart: func() error {
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
			tabplayersmutex.Lock()
			img := tabdrawer.DrawTab(tabplayers, tabtop, tabbottom, &tabparams)
			tabplayersmutex.Unlock()
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
		"lasttpssamples": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			buff := bytes.NewBufferString("")
			rows := noerr(db.Query(`select strftime('%Y-%m-%d %H:%M:%S', datetime(whenlogged, 'unixepoch')) , tpsvalue from tps order by whenlogged desc limit 50;`))
			fmt.Fprint(buff, "Last 50 TPS samples:\n")
			fmt.Fprint(buff, "Timestamp (UTC), TPS\n")
			for rows.Next() {
				var tmstp string
				var tpsval float64
				err := rows.Scan(&tmstp, &tpsval)
				if err != nil {
					log.Println(err)
					break
				}
				fmt.Fprintf(buff, "%s, %.2f\n", tmstp, tpsval)
			}
			rows.Close()
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Files: []*discordgo.File{{
						Name:        "tpslog.txt",
						ContentType: "text/plain",
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
	imagefetches := make(chan struct {
		i   uuid.UUID
		url string
	}, 512)
	go func() {
		for v := range imagefetches {
			textureresp, err := http.Get(v.url)
			if err != nil {
				log.Println("Error fetching ", v.url, err)
				continue
			}
			teximg, err := png.Decode(textureresp.Body)
			if err != nil {
				log.Println("Error decoding ", v.url, err)
				continue
			}
			var headimg image.Image
			headimg, _ = CropImage(teximg, image.Rect(8, 8, 16, 16))
			log.Println("GET " + v.url)
			tabplayersmutex.Lock()
			player, ok := tabplayers[v.i]
			if ok {
				player.HeadTexture = headimg
				tabplayers[v.i] = player
			}
			tabplayersmutex.Unlock()
		}
	}()
	client.Events.AddListener(bot.PacketHandler{
		ID:       packetid.ClientboundPlayerInfoRemove,
		Priority: 20,
		F: func(p pk.Packet) error {
			tabplayersmutex.Lock()
			defer tabplayersmutex.Unlock()
			r := bytes.NewReader(p.Data)
			var (
				length pk.VarInt
				id     pk.UUID
			)
			if _, err := length.ReadFrom(r); err != nil {
				return err
			}
			for i := 0; i < int(length); i++ {
				if _, err := id.ReadFrom(r); err != nil {
					return err
				}
				delete(tabplayers, uuid.UUID(id))
			}
			return nil
		}}, bot.PacketHandler{
		ID:       packetid.ClientboundPlayerInfoUpdate,
		Priority: 20,
		F: func(p pk.Packet) error {
			tabplayersmutex.Lock()
			defer tabplayersmutex.Unlock()
			r := bytes.NewReader(p.Data)
			action := pk.NewFixedBitSet(6)
			if _, err := action.ReadFrom(r); err != nil {
				log.Println(err)
				return err
			}
			var length pk.VarInt
			if _, err := length.ReadFrom(r); err != nil {
				log.Println(err)
				return err
			}
			for i := 0; i < int(length); i++ {
				var id pk.UUID
				if _, err := id.ReadFrom(r); err != nil {
					log.Println(err)
					return err
				}
				uid := uuid.UUID(id)
				player, ok := tabplayers[uid]
				if !ok { // create new player info if not exist
					player = tabdrawer.TabPlayer{}
				}
				// add player
				if action.Get(0) {
					var name pk.String
					var properties []user.Property
					if _, err := (pk.Tuple{&name, pk.Array(&properties)}).ReadFrom(r); err != nil {
						log.Println(err)
						return err
					}
					player.Name = chat.Text(string(name))
					for _, v := range properties {
						if v.Name != "textures" {
							continue
						}
						var tex map[string]interface{}
						must(json.Unmarshal(noerr(base64.StdEncoding.DecodeString(string(v.Value))), &tex))
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
						imagefetches <- struct {
							i   uuid.UUID
							url string
						}{
							i:   uid,
							url: texurl,
						}
						break
					}
				}
				if action.Get(1) {
					var chatSession pk.Option[sign.Session, *sign.Session]
					if _, err := chatSession.ReadFrom(r); err != nil {
						return err
					}
				}
				// update gamemode
				if action.Get(2) {
					var gamemode pk.VarInt
					if _, err := gamemode.ReadFrom(r); err != nil {
						log.Println(err)
						return err
					}
					if gamemode < 0 || gamemode > 3 {
						log.Printf("Overflow of the gamemode update: %#v", gamemode)
						continue
					}
					gmodes := []string{"Survival", "Creative", "Adventure", "Spectator"}
					if gamemode == 1 {
						log.Printf("Gamemode change %v %v", uuidToString(uid), gamemode)
						mtods <- discordgo.MessageSend{
							Content: fmt.Sprintf("Player %v changed game mode to %v", uuidToString(uid), gmodes[gamemode]),
							AllowedMentions: &discordgo.MessageAllowedMentions{
								Users: []string{"343418440423309314"},
							},
						}
					}
					player.Gamemode = gmodes[gamemode]
				}
				// update listed
				if action.Get(3) {
					var listed pk.Boolean
					if _, err := listed.ReadFrom(r); err != nil {
						log.Println(err)
						return err
					}
					if !listed {
						log.Printf("Someone not listed %v", uuidToString(uid))
						mtods <- discordgo.MessageSend{
							Content: fmt.Sprintf("Player %v is reported to not be listed in tab", uuidToString(uid)),
							AllowedMentions: &discordgo.MessageAllowedMentions{
								Users: []string{"343418440423309314"},
							},
						}
					}
				}
				// update latency
				if action.Get(4) {
					var latency pk.VarInt
					if _, err := latency.ReadFrom(r); err != nil {
						log.Println(err)
						return err
					}
					player.Ping = int(latency)
				}
				// display name
				if action.Get(5) {
					var displayName pk.Option[chat.Message, *chat.Message]
					if _, err := displayName.ReadFrom(r); err != nil {
						log.Println(err)
						return err
					}
					if displayName.Has {
						player.Name = displayName.Val
						if len(displayName.Val.Extra) != 1 {
							j, err := displayName.Val.MarshalJSON()
							log.Println(err, "Weird stuff", string(j))
						}
					}
				}
				// log.Println(printargs)
				tabplayers[uid] = player
			}
			return nil
		},
	})
	keepalivePackets := make(chan bool)
	client.Events.AddListener(bot.PacketHandler{
		ID:       packetid.ClientboundKeepAlive,
		Priority: 20,
		F: func(_ pk.Packet) error {
			keepalivePackets <- true
			return nil
		},
	})
	go func() {
		disconnectTime := 30 * time.Second
		disconnectTimer := time.NewTimer(disconnectTime)
		for {
			select {
			case <-disconnectTimer.C:
				client.Conn.Close()
			case <-keepalivePackets:
				if !disconnectTimer.Stop() {
					<-disconnectTimer.C
				}
				disconnectTimer.Reset(disconnectTime)
			}
		}
	}()
	for {
		timeout := time.Second * 20
		for k := range tabplayers {
			delete(tabplayers, k)
		}
		tabtop = &chat.Message{}
		tabbottom = &chat.Message{}
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
		dialer := net.Dialer{Timeout: timeout}
		mcdialer := (*mcnet.Dialer)(&dialer)
		err = client.JoinServerWithOptions(loadedConfig.ServerAddress, bot.JoinOptions{
			MCDialer:    mcdialer,
			Context:     dialctx,
			NoPublicKey: true,
			KeyPair:     nil,
		})
		dialctxcancel()
		if err != nil {
			mtod <- "Failed to join server: " + err.Error()
			time.Sleep(timeout)
			continue
		}
		err = client.HandleGame()
		if err != nil {
			mtod <- "Disconnected: " + err.Error()
			time.Sleep(timeout)
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
