package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"sort"
	"strings"

	"github.com/Tnze/go-mc/chat"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/disintegration/imaging"
	"github.com/fogleman/gg"
)

var chatColorCodes = map[string]color.RGBA{
	"black":        {0, 0, 0, 255},
	"dark_blue":    {0, 0, 170, 255},
	"dark_green":   {0, 170, 0, 255},
	"dark_aqua":    {0, 170, 170, 255},
	"dark_red":     {170, 0, 0, 255},
	"dark_purple":  {170, 0, 170, 255},
	"gold":         {255, 170, 0, 255},
	"gray":         {170, 170, 170, 255},
	"dark_gray":    {85, 85, 85, 255},
	"blue":         {85, 85, 255, 255},
	"green":        {85, 255, 85, 255},
	"aqua":         {85, 255, 255, 255},
	"red":          {255, 85, 85, 255},
	"light_purple": {255, 85, 255, 255},
	"yellow":       {255, 255, 85, 255},
	"white":        {255, 255, 255, 255},
}

type renderFragment struct {
	str   string
	color color.Color
	x, y  float64
}

func concatChatMessage(msg chat.Message) string {
	ret := msg.Text
	for _, e := range msg.Extra {
		ret += concatChatMessage(e)
	}
	return ret
}

func measureMaxLine(c *gg.Context, msg chat.Message) (w, h float64) {
	for _, s := range strings.Split(concatChatMessage(msg), "\n") {
		// if len(strings.ReplaceAll(s, " ", "")) == 0 {
		// 	continue
		// }
		ww, hh := c.MeasureString(s)
		if w < ww {
			w = ww
		}
		h += hh + 3
	}
	return
}

func measureChatLine(c *gg.Context, msg chat.Message) (ret bool, w, h float64) {
	strs := strings.Split(msg.Text, "\n")
	w, h = c.MeasureString(strs[0])
	if len(strs) > 1 {
		return true, w, h
	}
	for _, e := range msg.Extra {
		ret, ww, hh := measureChatLine(c, e)
		w += ww
		if ret {
			return true, w, h
		}
		if hh > h {
			h = hh
		}
	}
	return false, w, h
}

func fragmentMessage(c *gg.Context, align gg.Align, msg chat.Message, x, y float64) []renderFragment {
	lx := float64(0)
	lh := float64(0)
	return fragmentMultilineMessage(c, align, msg, &x, &y, &lx, &lh, 0, 0)
}

func fragmentMultilineMessage(c *gg.Context, align gg.Align, msg chat.Message, x, y, lx, lh *float64, law, lah float64) []renderFragment {
	col := color.RGBA{255, 255, 255, 255}
	if msg.Color != "" {
		coll, ok := chatColorCodes[msg.Color]
		if ok {
			col = coll
		}
	}
	c.SetColor(col)
	strs := strings.Split(msg.Text, "\n")
	fragments := []renderFragment{}
	for line := 0; line < len(strs)-1; line++ {
		w, h := c.MeasureString(strs[line])
		var xx float64
		switch align {
		case gg.AlignCenter:
			xx = *x - (*lx+w)/2 + *lx
		case gg.AlignRight:
			xx = *x + *lx
		case gg.AlignLeft:
			xx = *x - w - *lx
		}
		if *lh < h {
			*lh = h
		}
		fragments = append(fragments, renderFragment{
			str:   strs[line],
			color: col,
			x:     xx,
			y:     *y,
		})
		*y += *lh + 3
		*lx = 0
		*lh = 0
	}
	s := strs[len(strs)-1]
	if s != "" {
		w, h := c.MeasureString(s)
		tw := float64(0)
		for _, extra := range msg.Extra {
			brr, ew, eh := measureChatLine(c, extra)
			tw += ew
			if eh > h {
				h = eh
			}
			if brr {
				break
			}
		}
		if *lh < h {
			*lh = h
		}
		var xx float64
		switch align {
		case gg.AlignCenter:
			xx = *x - ((*lx + w + tw + law) / 2) + *lx
		case gg.AlignRight:
			xx = *x + *lx
		case gg.AlignLeft:
			xx = *x - w - *lx
		}
		fragments = append(fragments, renderFragment{
			str:   s,
			color: col,
			x:     xx,
			y:     *y,
		})
		*lx = *lx + w
	}
	for i := 0; i < len(msg.Extra); i++ {
		ew := float64(0)
		eh := float64(0)
		for j := i + 1; j < len(msg.Extra); j++ {
			brr, nw, nh := measureChatLine(c, msg.Extra[j])
			ew += nw
			if eh > nh {
				eh = nh
			}
			if brr {
				break
			}
		}
		fragments = append(fragments, fragmentMultilineMessage(c, align, msg.Extra[i], x, y, lx, lh, ew, eh)...)
	}
	return fragments
}

func getLatencyColor(ping int) color.Color {
	if ping < 60 {
		return color.RGBA{0, 255, 0, 255}
	} else if ping < 120 {
		return color.RGBA{105, 155, 0, 255}
	} else if ping < 240 {
		return color.RGBA{180, 90, 0, 255}
	} else if ping < 600 {
		return color.RGBA{255, 60, 60, 255}
	} else {
		return color.RGBA{255, 60, 60, 255}
	}
}

var (
	mctx = gg.NewContext(500, 500)
)

func drawTab(players map[pk.UUID]TabPlayer, tabtop, tabbottom *chat.Message) io.Reader {
	maxRows := 20
	colxspacing := float64(6)
	rowyspacing := float64(1)
	toppadding := float64(10)

	cols := len(players) / maxRows
	if len(players)%maxRows != 0 {
		cols++
	}

	keys := make([]pk.UUID, 0, len(players))
	for u := range players {
		keys = append(keys, u)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.Compare(players[keys[i]].name, players[keys[j]].name) < 0
	})

	fontsize := float64(31)
	mctx.LoadFontFace(loadedConfig.FontPath, fontsize)
	pmw, pmh := float64(0), float64(0)
	for _, v := range players {
		w, h := mctx.MeasureString(fmt.Sprint(v.name, v.ping, "    ms"))
		if pmw < w {
			pmw = w
		}
		if pmh < h {
			pmh = h
		}
	}
	tabw := float64(float64(cols)*(pmw+pmh+colxspacing) + 16)
	tabtopw, tabtoph := measureMaxLine(mctx, *tabtop)
	tabbottomw, tabbottomh := measureMaxLine(mctx, *tabbottom)
	_, lineh := mctx.MeasureString(" ")
	if tabw < tabtopw {
		tabw = tabtopw + 16
	}
	if tabw < tabbottomw {
		tabw = tabbottomw + 16
	}

	tabh := (lineh + toppadding) + tabtoph + tabbottomh + (rowyspacing+lineh)*(float64(maxRows)) + 16 + lineh*2
	c := gg.NewContext(int(tabw), int(tabh))
	c.SetColor(color.RGBA{R: 0x36, G: 0x39, B: 0x3f, A: 0xff})
	c.Clear()
	c.LoadFontFace(loadedConfig.FontPath, fontsize)

	topf := fragmentMessage(c, gg.AlignCenter, *tabtop, tabw/2, lineh+toppadding)
	topmy := float64(0)
	for _, f := range topf {
		c.SetColor(f.color)
		c.DrawString(f.str, f.x, f.y)
		if topmy < f.y {
			topmy = f.y
		}
	}

	plc := 0
	colw := pmw + pmh
	rowh := pmh + 4
	for col := 0; col < cols; col++ {
		for row := 0; row < maxRows; row++ {
			if plc > len(keys)-1 {
				break
			}
			pl := players[keys[plc]]
			c.SetColor(color.RGBA{0, 0, 0, 150})
			rowx := tabw/2 - (float64(cols)*(colw+colxspacing))/2 + float64(col)*(colw+colxspacing) + colxspacing/2
			rowy := topmy + lineh + float64(row)*(rowh+rowyspacing)
			c.DrawRectangle(rowx, rowy, colw, rowh)
			c.Fill()
			c.SetColor(color.White)
			c.DrawString(pl.name, rowx+rowh+1, rowy+rowh-3)
			pings := fmt.Sprintf("%dms", pl.ping)
			pw, _ := c.MeasureString(pings)
			c.SetColor(getLatencyColor(pl.ping))
			c.DrawString(pings, rowx+colw-pw, rowy+rowh-3)
			head, _ := CropImage(pl.texture, image.Rect(8, 8, 16, 16))
			c.DrawImage(imaging.Resize(head, int(rowh), int(rowh), imaging.NearestNeighbor), int(rowx), int(rowy))
			plc++
		}
	}
	bottomf := fragmentMessage(c, gg.AlignCenter, *tabbottom, tabw/2, topmy+lineh*2+rowh*float64(maxRows))
	for _, f := range bottomf {
		c.SetColor(f.color)
		c.DrawString(f.str, f.x, f.y)
	}

	buf := bytes.NewBufferString("")
	c.EncodePNG(buf)
	return buf
}

func CropImage(img image.Image, cropRect image.Rectangle) (cropImg image.Image, newImg bool) {
	//Interface for asserting whether `img`
	//implements SubImage or not.
	//This can be defined globally.
	type CropableImage interface {
		image.Image
		SubImage(r image.Rectangle) image.Image
	}

	if p, ok := img.(CropableImage); ok {
		// Call SubImage. This should be fast,
		// since SubImage (usually) shares underlying pixel.
		cropImg = p.SubImage(cropRect)
	} else if cropRect = cropRect.Intersect(img.Bounds()); !cropRect.Empty() {
		// If `img` does not implement `SubImage`,
		// copy (and silently convert) the image portion to RGBA image.
		rgbaImg := image.NewRGBA(cropRect)
		for y := cropRect.Min.Y; y < cropRect.Max.Y; y++ {
			for x := cropRect.Min.X; x < cropRect.Max.X; x++ {
				rgbaImg.Set(x, y, img.At(x, y))
			}
		}
		cropImg = rgbaImg
		newImg = true
	} else {
		// Return an empty RGBA image
		cropImg = &image.RGBA{}
		newImg = true
	}

	return cropImg, newImg
}
