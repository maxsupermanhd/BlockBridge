package main

import (
	"fmt"
	"log"
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
	noerr(dg.ApplicationCommandCreate(AppID, GuildID, &discordgo.ApplicationCommand{
		ID:            "tabCommand",
		ApplicationID: AppID,
		GuildID:       GuildID,
		Version:       "1",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "tab",
		Description:   "renders out tab",
	}))
	// noerr(dg.ApplicationCommandCreate(AppID, GuildID, &discordgo.ApplicationCommand{
	// 	ID:            "tpsCommand",
	// 	ApplicationID: AppID,
	// 	GuildID:       GuildID,
	// 	Version:       "1",
	// 	Type:          discordgo.ChatApplicationCommand,
	// 	Name:          "tps",
	// 	Description:   "renders out tps chart",
	// }))
	noerr(dg.ApplicationCommandCreate(AppID, GuildID, &discordgo.ApplicationCommand{
		ID:            "lasttpssamples",
		ApplicationID: AppID,
		GuildID:       GuildID,
		Version:       "1",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "lasttpssamples",
		Description:   "spews out last tps sample",
	}))
	must(dg.Open())
	return dg
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
					dg.ChannelMessageSend(cid, lastAggregate)
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
				dg.ChannelMessageSend(cid, lastAggregate)
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
		id := cfg.GetDString("343418440423309314", "ImportantPingID")
		_, err := dg.ChannelMessageSendComplex(cid, &discordgo.MessageSend{
			Content: "<@" + id + ">",
			Embed: &discordgo.MessageEmbed{
				Type:  discordgo.EmbedTypeRich,
				Title: msg,
			},
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Users: []string{id},
			},
		})
		if err != nil {
			log.Println(err)
		}
	}
}
