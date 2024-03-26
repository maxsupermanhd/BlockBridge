package main

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

type pingbackonlineEventType int

const (
	pingbackonlineEventTypeConnected pingbackonlineEventType = iota
	pingbackonlineEventTypeDisonnected
	pingbackonlineEventTypeHeartbeat
)

type pingbackonlineEvent struct {
	name pingbackonlineEventType
	when time.Time
}

var (
	pingbackonlineEvents = make(chan pingbackonlineEvent, 50)
)

func firePingbackonlineEvent(et pingbackonlineEventType) {
	pingbackonlineEvents <- pingbackonlineEvent{
		name: et,
		when: time.Now(),
	}
}

func pingbackonlineDelivery(dg *discordgo.Session) {
	sendlast := time.Now()
	sendinterval := time.Second * 8
	var whenconnected, whendisconnected *time.Time
	var nextping *pingbackonline
	sendnext := time.NewTicker(2 * time.Second)
	for {
		select {
		case e := <-pingbackonlineEvents:
			switch e.name {
			case pingbackonlineEventTypeConnected:
				log.Println("Pingbackonline event connected")
				ptr := time.Now()
				whenconnected = &ptr
				n, err := getNextPingbackonlineSub(db)
				if err != nil {
					log.Printf("Failed to get next pingbackonline sub: %s", err.Error())
				} else {
					nextping = n
				}
			case pingbackonlineEventTypeDisonnected:
				log.Println("Pingbackonline event disconnected")
				whenconnected = nil
				ptr := time.Now()
				whendisconnected = &ptr
			}
		case <-sendnext.C:
			if nextping == nil {
				// log.Println("Pingbackonline no next ping")
				continue
			}
			if whenconnected == nil {
				// log.Println("Pingbackonline no whenconnected")
				continue
			}
			if time.Since(sendlast) < sendinterval {
				// log.Println("Pingbackonline last send since too short")
				continue
			}
			if time.Since(*whenconnected) >= time.Duration(nextping.subtime)*time.Second {
				log.Printf("Sending DM to %s for subbed back online ping (%d seconds)", nextping.discorduserid, nextping.subtime)
				content := fmt.Sprintf("> :partying_face: `constantiam.net` is back online for `%s`", time.Since(*whenconnected).Round(time.Second))
				if whendisconnected != nil {
					content += fmt.Sprintf(" and was down for `%s`", time.Since(*whendisconnected).Round(time.Second))
				}
				err := sendDM(dg, nextping.dmchannelid, content)
				if err != nil {
					log.Printf("Failed to send DM: %s", err.Error())
				}
				sendlast = time.Now()
				_, err = removePingbackonlineSub(db, nextping.discorduserid)
				if err != nil {
					log.Printf("Failed to remove sub after DM: %s", err.Error())
				}
				n, err := getNextPingbackonlineSub(db)
				if err != nil {
					log.Printf("Failed to get next pingbackonline sub: %s", err.Error())
				} else {
					nextping = n
				}
				// } else {
				// 	log.Printf("Next DM subtime %s connectedsince %s", time.Duration(nextping.subtime)*time.Second, time.Since(*whenconnected))
			}
		}
	}
}
