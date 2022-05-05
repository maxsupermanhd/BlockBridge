package main

import (
	"flag"
	"log"
	"path"

	"github.com/maxsupermanhd/WebChunk/credentials"
	gmma "github.com/maxsupermanhd/go-mc-ms-auth"
)

var (
	out = flag.String("out", "./", "Where to write retrieved credentials, will be written as \"username.json\"")
	cid = flag.String("cid", "88650e7e-efee-4857-b9a9-cf580a00ef43", "Azure AppID")
)

func main() {
	log.Println("Starting up...")
	flag.Parse()
	ms, err := gmma.AuthMSdevice(*cid)
	if err != nil {
		log.Fatal(err)
	}
	s := &credentials.StoredMicrosoftCredentials{
		Microsoft:     ms,
		Minecraft:     gmma.MCauth{},
		MinecraftUUID: "",
	}
	log.Println("Getting XBOX Live token...")
	XBLa, err := gmma.AuthXBL(s.Microsoft.AccessToken)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Getting XSTS token...")
	XSTSa, err := gmma.AuthXSTS(XBLa)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Getting Minecraft token...")
	MCa, err := gmma.AuthMC(XSTSa)
	if err != nil {
		log.Fatal(err)
	}
	s.Minecraft = MCa
	log.Println("Getting Minecraft profile...")
	resauth, err := gmma.GetMCprofile(MCa.Token)
	if err != nil {
		log.Fatal(err)
	}
	resauth.AsTk = MCa.Token
	s.MinecraftUUID = resauth.UUID
	err = credentials.WriteCredentials(path.Join(*out, resauth.Name+".json"), s)
	if err != nil {
		log.Fatal(err)
	}
}
