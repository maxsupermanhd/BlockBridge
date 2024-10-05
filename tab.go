package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"log"

	"bytes"

	"github.com/google/uuid"
	chat762 "github.com/maxsupermanhd/go-vmc/v762/chat"
	"github.com/maxsupermanhd/go-vmc/v767/bot"
	"github.com/maxsupermanhd/go-vmc/v767/chat"
	"github.com/maxsupermanhd/go-vmc/v767/chat/sign"
	"github.com/maxsupermanhd/go-vmc/v767/data/packetid"
	"github.com/maxsupermanhd/tabdrawer"

	pk "github.com/maxsupermanhd/go-vmc/v767/net/packet"
	"github.com/maxsupermanhd/go-vmc/v767/yggdrasil/user"
)

var (
	nameOverrides = map[string]chat.Message{}
	tabparams     = tabdrawer.TabParameters{
		ColumnSpacing:       8,
		RowSpacing:          1,
		RowAdditionalHeight: 2,
		OverridePlayerName: func(u uuid.UUID) *chat762.Message {
			v, ok := nameOverrides[u.String()]
			if ok {
				vb, _ := v.MarshalJSON()
				var vr chat762.Message
				vr.UnmarshalJSON(vb)
				return &vr
			}
			return nil
		},
	}
	tabactions = make(chan tabaction, 512)
)

type tabaction struct {
	op   string
	uid  uuid.UUID
	data interface{}
	resp chan interface{}
}

func ProcessProps(properties []user.Property, uid uuid.UUID) {
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
		sc.GetSkinAsync(uid, texurl, func(i image.Image, err error) {
			if err != nil {
				log.Printf("Failed to get skin: %s", err.Error())
				return
			}
			tabactions <- tabaction{
				op:   "setTexture",
				uid:  uid,
				data: i,
			}
		})
		break
	}
}

func TabProcessor() {
	tab := map[uuid.UUID]PlayerInfo{}
	texturecache := map[uuid.UUID]image.Image{}
	tabtop := chat.Message{}
	tabbottom := chat.Message{}
	for r := range tabactions {
		switch r.op {
		case "add":
			profile := r.data.(GameProfile)
			tab[r.uid] = PlayerInfo{GameProfile: profile}
			ProcessProps(profile.Properties, r.uid)
		case "setPing":
			val := r.data.(int32)
			p, ok := tab[r.uid]
			if !ok {
				log.Println("Ghost ping set", uuidToString(r.uid), val)
				continue
			}
			p.Latency = val
			tab[r.uid] = p
		case "setName":
			p, ok := tab[r.uid]
			if !ok {
				log.Println("Ghost name set", uuidToString(r.uid), r.data)
				continue
			}
			val, ok := r.data.(*chat.Message)
			if ok {
				p.DisplayName = val
			} else {
				p.DisplayName = nil
			}
			tab[r.uid] = p
		case "delete":
			delete(tab, r.uid)
			delete(texturecache, r.uid)
		case "clear":
			tab = map[uuid.UUID]PlayerInfo{}
			texturecache = map[uuid.UUID]image.Image{}
			tabtop = chat.Message{}
			tabbottom = chat.Message{}
		case "setTexture":
			texturecache[r.uid] = r.data.(image.Image)
		case "draw":
			td := map[uuid.UUID]tabdrawer.TabPlayer{}
			tc := 0
			for k, v := range tab {
				tc++
				p := tabdrawer.TabPlayer{Ping: int(v.Latency)}
				p.HeadTexture = texturecache[k]
				if v.DisplayName != nil {
					dn, _ := v.DisplayName.MarshalJSON()
					json.Unmarshal(dn, &p.Name)
				} else {
					p.Name = chat762.Text(v.Name)
				}
				td[k] = p
			}
			if tabbottom.ClearString() == "" {
				tabbottom = chat.Text(fmt.Sprintf("[BlockBridge] %d players online", tc))
			}
			if tabtop.ClearString() == "" {
				tabtop = chat.Text(fmt.Sprintf("[BlockBridge] connected to %s", cfg.GetDString("localhost", "ServerAddress")))
			}
			var img image.Image
			if cfg.GetDBool(false, "ConvertTabColorCodes") {
				ctabtop := tabdrawer.ConvertColorCodes(tabtop.Text)
				ctabbottom := tabdrawer.ConvertColorCodes(tabbottom.Text)
				img = tabdrawer.DrawTab(td, &ctabtop, &ctabbottom, &tabparams)
			} else {
				tbt, _ := tabtop.MarshalJSON()
				tbb, _ := tabbottom.MarshalJSON()
				var tt, tb chat762.Message
				tt.UnmarshalJSON(tbt)
				tb.UnmarshalJSON(tbb)
				img = tabdrawer.DrawTab(td, &tt, &tb, &tabparams)
			}
			r.resp <- img
		case "setTopBottom":
			tb := r.data.(struct{ top, bottom chat.Message })
			tabtop = tb.top
			tabbottom = tb.bottom
		case "snapshot":
			keys := make([]uuid.UUID, 0, len(tab))
			for u := range tab {
				keys = append(keys, u)
			}
			sn, err := json.Marshal(keys)
			if err != nil {
				log.Println("Failed to marshal for snapshot ", err)
			}
			r.resp <- string(sn)
		case "count":
			r.resp <- len(tab)
		default:
			log.Println("Unknown action")
			log.Printf("%#+v", r)
		}
	}
}

func addTabHandlers(client *bot.Client) {
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
}

func handleTabHeaderFooter(p pk.Packet) error {
	tabtop := chat.Message{}
	tabbottom := chat.Message{}
	must(p.Scan(&tabtop, &tabbottom))
	tabactions <- tabaction{
		op: "setTopBottom",
		data: struct{ top, bottom chat.Message }{
			top:    tabtop,
			bottom: tabbottom,
		},
	}
	// log.Println(string(noerr(tabtop.MarshalJSON())))
	// log.Println(string(noerr(tabbottom.MarshalJSON())))
	return nil
}

func handlePlayerInfoUpdatePacket(p pk.Packet) error {
	r := bytes.NewReader(p.Data)

	action := pk.NewFixedBitSet(6)
	if _, err := action.ReadFrom(r); err != nil {
		return err
	}

	var length pk.VarInt
	if _, err := length.ReadFrom(r); err != nil {
		return err
	}

	for i := 0; i < int(length); i++ {
		var id pk.UUID
		if _, err := id.ReadFrom(r); err != nil {
			return err
		}
		uid := uuid.UUID(id)

		// add player
		if action.Get(0) {
			var name pk.String
			var properties []user.Property
			if _, err := (pk.Tuple{&name, pk.Array(&properties)}).ReadFrom(r); err != nil {
				return err
			}
			tabactions <- tabaction{
				op:  "add",
				uid: uid,
				data: GameProfile{
					ID:         uid,
					Name:       string(name),
					Properties: properties,
				},
			}
		}
		// initialize chat
		if action.Get(1) {
			var chatSession pk.Option[sign.Session, *sign.Session]
			if _, err := chatSession.ReadFrom(r); err != nil {
				return err
			}
			// if chatSession.Has {
			// 	player.ChatSession = chatSession.Pointer()
			// 	player.ChatSession.InitValidate()
			// } else {
			// 	player.ChatSession = nil
			// }
		}
		// update gamemode
		if action.Get(2) {
			var gamemode pk.VarInt
			if _, err := gamemode.ReadFrom(r); err != nil {
				return err
			}
			if gamemode != 0 {
				log.Printf("Gamemode change %v %v", uuidToString(uid), gamemode)
				mtods <- fmt.Sprintf("Player %v changed game mode to %v", uuidToString(uid), gmodes[gamemode])
			}
		}
		// update listed
		if action.Get(3) {
			var listed pk.Boolean
			if _, err := listed.ReadFrom(r); err != nil {
				return err
			}
			if !listed {
				log.Printf("Someone not listed %v", uuidToString(uid))
				mtods <- fmt.Sprintf("Player %v became unlisted", uuidToString(uid))
			}
		}
		// update latency
		if action.Get(4) {
			var latency pk.VarInt
			if _, err := latency.ReadFrom(r); err != nil {
				return err
			}
			tabactions <- tabaction{
				op:   "setPing",
				uid:  uid,
				data: int32(latency),
			}
		}
		// display name
		if action.Get(5) {
			var displayName pk.Option[chat.Message, *chat.Message]
			if _, err := displayName.ReadFrom(r); err != nil {
				return err
			}
			if displayName.Has {
				tabactions <- tabaction{
					op:   "setName",
					uid:  uid,
					data: &displayName.Val,
				}
			} else {
				tabactions <- tabaction{
					op:   "setName",
					uid:  uid,
					data: nil,
				}
			}
		}
	}

	return nil
}

func handlePlayerInfoRemovePacket(p pk.Packet) error {
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
		tabactions <- tabaction{
			op:  "delete",
			uid: uuid.UUID(id),
		}
	}
	return nil
}

type PlayerInfo struct {
	GameProfile
	ChatSession *sign.Session
	Gamemode    int32
	Latency     int32
	Listed      bool
	DisplayName *chat.Message
}

type GameProfile struct {
	ID         uuid.UUID
	Name       string
	Properties []user.Property
}
