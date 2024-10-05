package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func handleDiscordMessage(_ *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	ChannelIDs := strings.Split(cfg.GetDSString("", "ChannelID"), ",")
	correctChannel := false
	for _, v := range ChannelIDs {
		if m.ChannelID == v {
			correctChannel = true
			break
		}
	}
	if !correctChannel {
		return
	}

	log.Printf("d->m [%v] [%v]", m.Author.Username, m.Content)

	toSend := m.Content
	if cfg.GetDSBool(false, "AddPrefix") {
		toSend = fmt.Sprintf("[Discord] %v#%v: %v", m.Author.Username, m.Author.Discriminator, m.Content)
	}

	dtom <- struct {
		msg    string
		userid string
	}{
		msg:    toSend,
		userid: m.Author.ID,
	}
}

func OpenDiscord() *discordgo.Session {
	log.Println("Connecting to discord...")
	DiscordToken, ok := cfg.GetString("Discord", "Token")
	if !ok {
		log.Fatal("Discord token was not found in config (\"DiscordConfig\")")
	}
	AppID, ok := cfg.GetString("Discord", "AppID")
	if !ok {
		log.Fatal("AppID was not found in config")
	}
	GuildID, ok := cfg.GetString("Discord", "GuildID")
	if !ok {
		log.Fatal("AppID was not found in config")
	}
	dg := noerr(discordgo.New("Bot " + DiscordToken))
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent
	dg.AddHandler(handleDiscordMessage)
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Discord connection ready")
	})
	if cfg.GetDBool(false, "Discord", "DeleteGlobalCommands") {
		log.Println("Fetching global commands to delete")
		cmds := noerr(dg.ApplicationCommands(AppID, ""))
		log.Printf("Fetched %d global commands", len(cmds))
		for _, v := range cmds {
			log.Println("Deleting global command ", v.ID)
			must(dg.ApplicationCommandDelete(AppID, "", v.ID))
		}
	}
	if cfg.GetDBool(false, "Discord", "DeleteGuildCommands") {
		log.Println("Fetching guild commands to delete")
		cmds := noerr(dg.ApplicationCommands(AppID, GuildID))
		log.Printf("Fetched %d guild commands", len(cmds))
		for _, v := range cmds {
			log.Println("Deleting guild command ", v.ID)
			must(dg.ApplicationCommandDelete(AppID, GuildID, v.ID))
		}
	}
	if cfg.GetDBool(true, "Discord", "InitGuildCommands") {
		log.Println("Initializing guild tab command")
		noerr(dg.ApplicationCommandCreate(AppID, GuildID, &discordgo.ApplicationCommand{
			ApplicationID: AppID,
			GuildID:       GuildID,
			Version:       "69",
			Type:          discordgo.ChatApplicationCommand,
			Name:          "tab",
			Description:   "renders out tab",
		}))
		log.Println("Initializing guild lasttpssamples command")
		noerr(dg.ApplicationCommandCreate(AppID, GuildID, &discordgo.ApplicationCommand{
			ApplicationID: AppID,
			GuildID:       GuildID,
			Version:       "69",
			Type:          discordgo.ChatApplicationCommand,
			Name:          "lasttpssamples",
			Description:   "spews out last tps samples",
		}))
	}
	if cfg.GetDBool(true, "Discord", "InitGlobalCommands") {
		log.Println("Initializing global tab command")
		noerr(dg.ApplicationCommandCreate(AppID, "", &discordgo.ApplicationCommand{
			ApplicationID: AppID,
			Version:       "69",
			Type:          discordgo.ChatApplicationCommand,
			Name:          "tab",
			Description:   "renders out tab",
		}))
		log.Println("Initializing global lasttpssamples command")
		noerr(dg.ApplicationCommandCreate(AppID, "", &discordgo.ApplicationCommand{
			ApplicationID: AppID,
			Version:       "69",
			Type:          discordgo.ChatApplicationCommand,
			Name:          "lasttpssamples",
			Description:   "spews out last tps samples",
		}))
	}
	must(dg.Open())
	return dg
}

var (
	componentHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"pingbackonline_select_time": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Select amount of time server has to be online.\n" +
						"Pings will be sent out to DMs, please allow them if you want to get it.\n" +
						"Once bot joins the server and stays on it connected (1m or so disconnect timeout counts as being on the server)\n" +
						"Every selected time it will check who subbed to specific times and will start delivering DMs.\n" +
						"If you sub to ping when server is online then you will get a ping once Yokai0nTop reconnects to the server (either bot/server restarts or internet dies on either end).",
					Flags: discordgo.MessageFlagsEphemeral,
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								discordgo.SelectMenu{
									MenuType: discordgo.StringSelectMenu,
									CustomID: "pingbackonline_selected",
									Options: []discordgo.SelectMenuOption{
										{Emoji: discordgo.ComponentEmoji{Name: "🏓"}, Label: "Immediately", Value: "0"},
										{Emoji: discordgo.ComponentEmoji{Name: "🏓"}, Label: "1 minute", Value: "60"},
										{Emoji: discordgo.ComponentEmoji{Name: "🏓"}, Label: "2 minutes", Value: "120"},
										{Emoji: discordgo.ComponentEmoji{Name: "🏓"}, Label: "3 minutes", Value: "180"},
										{Emoji: discordgo.ComponentEmoji{Name: "🏓"}, Label: "5 minutes", Value: "300"},
										{Emoji: discordgo.ComponentEmoji{Name: "🏓"}, Label: "10 minutes", Value: "600"},
										{Emoji: discordgo.ComponentEmoji{Name: "🏓"}, Label: "15 minutes", Value: "900"},
										{Emoji: discordgo.ComponentEmoji{Name: "🏓"}, Label: "20 minutes", Value: "1200"},
										{Emoji: discordgo.ComponentEmoji{Name: "❌"}, Label: "Unsubscribe", Value: "-1"},
									},
								},
							},
						},
					},
				},
			})
			if err != nil {
				log.Printf("Failed to respond with interaction: %s", err.Error())
			}
		},
		"pingbackonline_selected": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			subtime, err := strconv.Atoi(i.MessageComponentData().Values[0])
			if err != nil {
				iRespondUpdate(s, i, fmt.Sprintf("Invalid time from selector (bug?): %q", err.Error()))
				return
			}
			if subtime < 0 {
				log.Printf("Removing backonline sub from %s %s", i.Member.User.Username, i.Member.User.ID)
				result, err := removePingbackonlineSub(db, i.Member.User.ID)
				if err != nil {
					iRespondUpdate(s, i, fmt.Sprintf("Error removing sub from database: %q (result %q)\n", err.Error(), result))
					return
				}
				iRespondUpdate(s, i, fmt.Sprintf("Your username is %q, id is %q and you selected removal of the ping\n"+
					"Your ping subscription was removed, you will not be DMed.", i.Member.User.Username, i.Member.User.ID))
			} else {
				dmchannelid, err := openDM(s, i.Member.User.ID)
				if err != nil {
					iRespondUpdate(s, i, fmt.Sprintf("Can't open a DM to you, please check if it is allowed (%s)", err.Error()))
					return
				}
				result, err := recordPingbackonlineSub(db, pingbackonline{
					discorduserid: i.Member.User.ID,
					dmchannelid:   dmchannelid,
					subtime:       subtime,
				})
				if err != nil {
					iRespondUpdate(s, i, fmt.Sprintf("Error recording sub to database: %q (result %q)\n", err.Error(), result))
					return
				}
				log.Printf("New backonline sub from %s %s for %s seconds", i.Member.User.Username, i.Member.User.ID, i.MessageComponentData().Values[0])
				iRespondUpdate(s, i, fmt.Sprintf("Your username is %q, id is %q and you selected %q seconds\n"+
					"Your ping subscription was recorded, if you want to cancel it, select unsubscribe option.", i.Member.User.Username, i.Member.User.ID, i.MessageComponentData().Values[0]))
			}
		},
		"intTab": intRespTab,
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"tab":            intRespTab,
		"lasttpssamples": intRespLastTpsSamples,
	}
)

func intRespTab(s *discordgo.Session, i *discordgo.InteractionCreate) {
	rsp := make(chan interface{})
	tabactions <- tabaction{
		op:   "draw",
		resp: rsp,
	}
	buff := bytes.NewBufferString("")
	must(png.Encode(buff, (<-rsp).(image.Image)))
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
			Files: []*discordgo.File{{
				Name:        "tab.png",
				ContentType: "image/png",
				Reader:      buff,
			}},
		},
	})
}

func intRespLastTpsSamples(s *discordgo.Session, i *discordgo.InteractionCreate) {
	iRespondLoading(s, i, "Getting tps data...")
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Files: []*discordgo.File{{
			Name:        "tpslog.txt",
			ContentType: "text/plain",
			Reader:      GetLastTPSValues(db),
		}},
	})
}

func iRespondLoading(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: content,
		},
	})
	if err != nil {
		log.Printf("Failed to respond to interaction with loading: %q", err.Error())
		debug.PrintStack()
	}
}

func iRespondUpdate(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{Content: content},
	})
	if err != nil {
		log.Printf("Failed to respond to interaction with message update: %q", err.Error())
		debug.PrintStack()
	}
}

func openDM(dg *discordgo.Session, discorduserid string) (string, error) {
	ch, err := dg.UserChannelCreate(discorduserid)
	return ch.ID, err
}

func sendDM(dg *discordgo.Session, dmid string, content string) error {
	_, err := dg.ChannelMessageSend(dmid, content)
	return err
}

func pipeMessagesToDiscord(dg *discordgo.Session) {
	lastMessage := time.Now()
	lastAggregate := ""
	dflusher := time.NewTicker(time.Second * 5)
	for {
		select {
		case msg := <-mtod:
			msg = strings.ReplaceAll(msg, "_", "\\_")
			msg = strings.ReplaceAll(msg, "`", "\\`")
			msg = strings.ReplaceAll(msg, "*", "\\*")
			msg = strings.ReplaceAll(msg, "~", "\\~")
			msg = strings.ReplaceAll(msg, "#", "\\#")
			msg = strings.ReplaceAll(msg, "-", "\\-")
			msg = strings.ReplaceAll(msg, "|", "\\|")
			if cfg.GetDBool(false, "AddTimestamps") {
				msg = time.Now().Format("`[02 Jan 06 15:04:05]` ") + msg
			}
			log.Printf("m-|d [%v]", msg)
			lastAggregate += msg + "\n"
			if time.Since(lastMessage).Seconds() > 2.5*float64(time.Second) {
				cid, ok := cfg.GetString("ChannelID")
				if ok {
					dg.ChannelMessageSendComplex(cid, &discordgo.MessageSend{
						Content: lastAggregate,
						AllowedMentions: &discordgo.MessageAllowedMentions{
							Parse:       []discordgo.AllowedMentionType{},
							Roles:       []string{},
							Users:       []string{},
							RepliedUser: false,
						},
					})
				}
				lastAggregate = ""
				lastMessage = time.Now()
			}
		case <-dflusher.C:
			if lastAggregate == "" {
				continue
			}
			cid, ok := cfg.GetString("ChannelID")
			if ok {
				dg.ChannelMessageSendComplex(cid, &discordgo.MessageSend{
					Content: lastAggregate,
					AllowedMentions: &discordgo.MessageAllowedMentions{
						Parse:       []discordgo.AllowedMentionType{},
						Roles:       []string{},
						Users:       []string{},
						RepliedUser: false,
					},
				})
			}
			lastAggregate = ""
			lastMessage = time.Now()
		}
	}
}

func pipeImportantMessagesToDiscord(dg *discordgo.Session) {
	for msg := range mtods {
		cid, ok := cfg.GetString("ChannelID")
		if !ok {
			log.Println("ChannelID is not set, not sending ping message")
			continue
		}
		log.Println("Ping event: ", msg)
		id := cfg.GetDString("343418440423309314", "ImportantPingID")
		_, err := dg.ChannelMessageSendComplex(cid, &discordgo.MessageSend{
			Content: "<@" + id + ">",
			Embeds: []*discordgo.MessageEmbed{{
				Title:       "1fd84914aba013d0f57915b3024dbb623a3e68fd2ec2ed4200233d51e1e91e03",
				Description: msg,
				Color:       392960,
			}},
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Users: []string{id},
			},
		})
		if err != nil {
			log.Println(err)
		}
	}
}
