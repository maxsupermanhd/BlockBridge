package main

import (
	"fmt"
	"log"
	"strings"

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
	noerr(dg.ApplicationCommandCreate(AppID, GuildID, &discordgo.ApplicationCommand{
		ID:            "tabCommand",
		ApplicationID: AppID,
		GuildID:       GuildID,
		Version:       "1",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "tab",
		Description:   "renders out tab",
	}))
	noerr(dg.ApplicationCommandCreate(AppID, GuildID, &discordgo.ApplicationCommand{
		ID:            "tpsCommand",
		ApplicationID: AppID,
		GuildID:       GuildID,
		Version:       "1",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "tps",
		Description:   "renders out tps chart",
	}))
	noerr(dg.ApplicationCommandCreate(AppID, GuildID, &discordgo.ApplicationCommand{
		ID:            "lasttpssamples",
		ApplicationID: AppID,
		GuildID:       GuildID,
		Version:       "1",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "lasttpssamples",
		Description:   "spews out last tps sample",
	}))
	noerr(dg.ApplicationCommandCreate(AppID, GuildID, &discordgo.ApplicationCommand{
		ID:            "lagspikes",
		ApplicationID: AppID,
		GuildID:       GuildID,
		Version:       "2",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "lagspikes",
		Description:   "spews out last lagspikes and who was online",
		Options: []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "duration",
			Description: "Valid time units are 'ns', 'us' (or 'µs'), 'ms', 's', 'm', 'h'.",
			Required:    false,
			MinValue:    new(float64),
		}},
	}))
	must(dg.Open())
	return dg
}
