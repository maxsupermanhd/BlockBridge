package main

import (
	"log"
	"strings"

	"github.com/maxsupermanhd/go-vmc/v767/bot"
	"github.com/maxsupermanhd/go-vmc/v767/bot/basic"
	"github.com/maxsupermanhd/go-vmc/v767/bot/msg"
	"github.com/maxsupermanhd/go-vmc/v767/chat"
	"github.com/maxsupermanhd/go-vmc/v767/data/packetid"
	pk "github.com/maxsupermanhd/go-vmc/v767/net/packet"
)

var (
	botBasicSettings = basic.Settings{
		Locale:              "ru_RU",
		ViewDistance:        15,
		ChatMode:            0,
		DisplayedSkinParts:  basic.Jacket | basic.LeftSleeve | basic.RightSleeve | basic.LeftPantsLeg | basic.RightPantsLeg | basic.Hat,
		MainHand:            1,
		EnableTextFiltering: false,
		AllowListing:        true,
		Brand:               "BlockBridge",
		ChatColors:          true,
	}
	botBasicEvents = basic.EventsListener{
		GameStart: func() error {
			log.Println("Logged in")
			mtod <- "Logged in"
			return nil
		},
		Disconnect: func(reason chat.Message) error {
			r := reason.ClearString()
			log.Println("Disconnect: ", r)
			mtod <- "Disconnect: " + r
			if ds, ok := cfg.GetString("RegularDisconnectPacket"); ok && r != ds {
				mtods <- "Irregular Disconnect packet"
			}
			return nil
		},
		HealthChange: nil,
		Death:        nil,
	}
	botMessageEvents = msg.EventsHandler{
		// TODO
		// SystemChat: func(msg chat.Message, overlay bool) error {
		// 	if msg.ClearString() == "*** Server maintenance in 1 minute ***" {
		// 		dtom <- struct {
		// 			msg    string
		// 			userid string
		// 		}{
		// 			msg:    "Subscribe for a ping in my discord server to get automatically notified when server comes back online",
		// 			userid: "feedback",
		// 		}
		// 	}
		// 	if !overlay {
		// 		mtod <- msg.ClearString()
		// 	}
		// 	return nil
		// },
		PlayerChatMessage: func(msg chat.Message, _ bool) error {
			mtod <- msg.ClearString()
			return nil
		},
		DisguisedChat: func(msg chat.Message) error {
			mtod <- msg.ClearString()
			return nil
		},
	}
)

func pipeMessagesFromDiscord(client *bot.Client, msgman *msg.Manager) {
	for m := range dtom {
		allowedsend := true
		allowList, ok := cfg.GetString("AllowedChat")
		if ok {
			allowedsend = false
			for _, allowedid := range strings.Split(allowList, ",") {
				if m.userid == allowedid {
					allowedsend = true
					log.Println("message from ", m.userid, " was whitelisted")
					break
				}
			}
			if m.userid == "feedback" {
				allowedsend = true
			}
		}
		if !allowedsend {
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
			allowList, ok = cfg.GetString("", "AllowedSlash")
			if ok {
				for _, allowedid := range strings.Split(allowList, ",") {
					if m.userid == allowedid {
						allowedsend = true
						break
					}
				}
			} else {
				allowedsend = true
			}
			if allowedsend {
				// log.Println([]byte(m[1:]))
				client.Conn.WritePacket(pk.Marshal(packetid.ServerboundChatCommand,
					pk.String(m.msg[1:]),
				))
			}
		} else {
			msgman.SendMessage(m.msg)
		}

	}
}
