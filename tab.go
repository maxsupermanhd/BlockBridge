package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"log"
	"net/http"

	"bytes"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/chat/sign"
	"github.com/google/uuid"
	"github.com/maxsupermanhd/tabdrawer"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil/user"
)

var (
	nameOverrides = map[string]chat.Message{}
	tabparams     = tabdrawer.TabParameters{
		ColumnSpacing:       8,
		RowSpacing:          1,
		RowAdditionalHeight: 2,
		OverridePlayerName: func(u uuid.UUID) *chat.Message {
			v, ok := nameOverrides[u.String()]
			if ok {
				return &v
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
		log.Println("Scheduled head image fetch for ", uuidToString(uid))
		go func(url string, uid uuid.UUID) {
			textureresp, err := http.Get(url)
			if err != nil {
				log.Println("Error fetching ", url, err)
				return
			}
			teximg, err := png.Decode(textureresp.Body)
			if err != nil {
				log.Println("Error decoding ", url, err)
				return
			}
			var headimg image.Image
			headimg, _ = CropImage(teximg, image.Rect(8, 8, 16, 16))
			log.Println("GET " + url)
			tabactions <- tabaction{
				op:   "setTexture",
				uid:  uid,
				data: headimg,
			}
		}(texurl, uid)
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
			for k, v := range tab {
				p := tabdrawer.TabPlayer{Ping: int(v.Latency)}
				p.HeadTexture = texturecache[k]
				if v.DisplayName != nil {
					p.Name = *v.DisplayName
				} else {
					p.Name = chat.Text(v.Name)
				}
				td[k] = p
			}
			img := tabdrawer.DrawTab(td, &tabtop, &tabbottom, &tabparams)
			r.resp <- img
		case "setTopBottom":
			tb := r.data.(struct{ top, bottom chat.Message })
			tabtop = tb.top
			tabbottom = tb.bottom
		default:
			log.Println("Unknown action")
			log.Printf("%#+v", r)
		}
	}
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
