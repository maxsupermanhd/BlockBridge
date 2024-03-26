package main

import (
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/bot/basic"
	"github.com/Tnze/go-mc/bot/msg"
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
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
			log.Println("Disconnect: ", reason.String())
			mtod <- "Disconnect: " + reason.ClearString()
			return nil
		},
		HealthChange: nil,
		Death:        nil,
	}
	botMessageEvents = msg.EventsHandler{
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
}
