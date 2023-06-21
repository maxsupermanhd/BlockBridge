package main

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

func handleDiscordMessage(_ *discordgo.Session, m *discordgo.MessageCreate) {
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
}

func OpenDiscord() *discordgo.Session {
	log.Println("Connecting to discord...")
	dg := noerr(discordgo.New("Bot " + loadedConfig.DiscordToken))
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent
	dg.AddHandler(handleDiscordMessage)
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
		ID:            "lasttpssamples",
		ApplicationID: loadedConfig.DiscordAppID,
		GuildID:       loadedConfig.DiscordGuildID,
		Version:       "1",
		Type:          discordgo.ChatApplicationCommand,
		Name:          "lasttpssamples",
		Description:   "spews out last tps sample",
	}))
	must(dg.Open())
	return dg
}
