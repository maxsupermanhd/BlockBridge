package main

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"log"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/maxsupermanhd/tpsdrawer"
	"github.com/mazznoer/colorgrad"
)

var (
	cachedStatusMessageID string
)

func measurePercentile(c []float64) (percentile float64) {
	percent := 1.0
	if len(c) == 0 {
		return 0
	}
	if len(c) == 1 {
		return c[0]
	}
	sort.Float64s(c)
	index := (percent / 100) * float64(len(c))
	if index == float64(int64(index)) {
		i := int(index)
		return c[i-1]
	} else if index > 1 {
		i := int(index)
		return c[i-1] + c[i]/float64(len(c))
	} else {
		return 0
	}
}

func getStatusTPS(db *pgxpool.Pool) (io.Reader, io.Reader, string, error) {
	profilerbegin := time.Now()
	tpsval, tpsn, plc, err := GetTPSPlayercountValues(db, nil)
	if err != nil {
		log.Println(err)
		return nil, nil, "", err
	}
	if len(tpsval) == 0 {
		return nil, nil, "", errors.New("nothing to draw")
	}
	profilerGotData := time.Since(profilerbegin).Round(time.Second / 10)
	img := drawTPS(tpsval, tpsn, plc)
	profilerChartDrawn := time.Since(profilerbegin).Round(time.Second / 10)
	t := time.Duration(30 * 24 * time.Hour)
	tpsval, tpsn, err = GetTPSValues(db, &t)
	if err != nil {
		log.Println(err)
		return nil, nil, "", err
	}
	if len(tpsval) == 0 {
		return nil, nil, "", errors.New("nothing to draw")
	}
	grad := noerr(colorgrad.NewGradient().
		HtmlColors("darkred", "gold", "green").
		Domain(0, 20).
		Build())
	img2 := tpsdrawer.DrawTPS(tpsval, tpsn, tpsdrawer.DrawOptions{
		DayW:       100,
		DayH:       40,
		Padding:    8,
		Spacing:    4,
		Background: color.RGBA{R: 0x36, G: 0x39, B: 0x3f, A: 0xff},
		FontColor:  color.White,
		Gradient: func(f float64) color.Color {
			if f == 0 {
				return color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xFF}
			}
			r, g, b := grad.At(f).RGB255()
			return color.RGBA{R: r, G: g, B: b, A: 0xFF}
		},
		SampleH:     32,
		Comment:     fmt.Sprint("Made by FlexCoral, tracked by Yokai0nTop, ", len(tpsval), " samples"),
		BreakMonths: true,
		BreakMonday: true,
		MeasureFunc: measurePercentile,
	})
	profilerHeatmapDrawn := time.Since(profilerbegin).Round(time.Second / 10)
	img2w := bytes.NewBufferString("")
	err = png.Encode(img2w, img2)
	if err != nil {
		log.Println(err)
		mtods <- err.Error()
	}
	cnt := fmt.Sprintf(`Got data: %s
Chart drawn: %s
Heatmap drawn: %s
Total: %s
Samples: %d`, profilerGotData, profilerChartDrawn, profilerHeatmapDrawn, time.Since(profilerbegin).Round(time.Second/10), len(tpsval))
	return img, img2w, cnt, nil
}

func getStatusPlayercount(db *pgxpool.Pool) (io.Reader, error) {
	t, p, err := getAvgPlayercountLong(db)
	if err != nil {
		return nil, err
	}
	return drawAvgPlayercount(t, p), nil
}

func updateStatus(dg *discordgo.Session, db *pgxpool.Pool) {
	channelID, ok := cfg.GetString("StatusChannelID")
	if !ok {
		return
	}
	if cachedStatusMessageID == "" {
		log.Println("No cached status message ID, loading channel messages")
		msgs, err := dg.ChannelMessages(channelID, 100, "", "", "")
		if err != nil {
			log.Printf("Failed to load discord channel %q messages: %s", channelID, err.Error())
			return
		}
		for _, v := range msgs {
			if v.Author.ID == dg.State.User.ID {
				cachedStatusMessageID = v.ID
				log.Printf("Found message to edit, new cached status message ID is %q", cachedStatusMessageID)
			}
		}
		if cachedStatusMessageID == "" {
			log.Printf("Message was not found, will send new one")
		} else {
			log.Printf("Found message %s", cachedStatusMessageID)
		}
	}
	log.Println("Status generating")
	tpschart, tpsheat, profiler, err := getStatusTPS(db)
	files := []*discordgo.File{{
		Name:        "tpsChart.png",
		ContentType: "image/png",
		Reader:      tpschart,
	}, {
		Name:        "tpsHeat.png",
		ContentType: "image/png",
		Reader:      tpsheat,
	}}
	content := profiler
	if err != nil {
		content = err.Error()
		files = []*discordgo.File{}
	}
	playercount, err := getStatusPlayercount(db)
	if err == nil {
		files = append(files, &discordgo.File{
			Name:        "avgPlayercount.png",
			ContentType: "image/png",
			Reader:      playercount,
		})
	} else {
		content += "\n\n" + err.Error()
	}
	if cachedStatusMessageID == "" {
		log.Println("No cached message ID, sending new one")
		msg, err := dg.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Content: content,
			Files:   files,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Ping when server comes back online",
							Style:    discordgo.PrimaryButton,
							CustomID: "pingbackonline_select_time",
							Emoji: discordgo.ComponentEmoji{
								Name: "🏓",
							},
						},
					},
				},
			},
		})
		if err != nil {
			log.Printf("Error sending status message: %q", err.Error())
		} else {
			cachedStatusMessageID = msg.ID
			log.Printf("New cached status message ID is %q", cachedStatusMessageID)
		}
	} else {
		log.Printf("Editing message ID %q", cachedStatusMessageID)
		_, err = dg.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Content:     &content,
			Files:       files,
			Attachments: &[]*discordgo.MessageAttachment{},
			ID:          cachedStatusMessageID,
			Channel:     channelID,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Ping when server comes back online",
							Style:    discordgo.PrimaryButton,
							CustomID: "pingbackonline_select_time",
							Emoji: discordgo.ComponentEmoji{
								Name: "🏓",
							},
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Get tab",
							Style:    discordgo.PrimaryButton,
							CustomID: "intTab",
							Emoji: discordgo.ComponentEmoji{
								Name: "↔️",
							},
						},
					},
				},
			},
		})
		if err != nil {
			log.Printf("Error editing cached message: %q, clearing cached message ID", err.Error())
			cachedStatusMessageID = ""
		}
	}
	log.Println("Status update done")
}

func statusUpdater(dg *discordgo.Session, db *pgxpool.Pool) {
	updateIntervalString, ok := cfg.GetString("StatusUpdateInterval")
	if !ok {
		return
	}
	updateInterval, err := time.ParseDuration(updateIntervalString)
	if err != nil {
		log.Printf("Error parsing StatusUpdateInterval: %s", err.Error())
	}
	for {
		log.Println("Performing status update...")
		updateStatus(dg, db)

		updateAt := time.Now().Add(updateInterval/2 + 2*time.Second).Round(updateInterval).Add(10 * time.Second)
		toWait := time.Until(updateAt)
		log.Printf("Next update at %s (in %s)", updateAt.Format("02 Jan 06 15:04:05"), toWait)
		<-time.After(toWait)
	}
}
