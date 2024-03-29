package skincache

import (
	"bytes"
	"encoding/hex"
	"image"
	"image/png"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/maxsupermanhd/lac"
)

type skinRequest struct {
	ret func(image.Image, error)
	id  uuid.UUID
	url string
}

type SkinCache struct {
	cfg      *lac.ConfSubtree
	requests chan *skinRequest
}

func NewSkinCache(cfg *lac.ConfSubtree) *SkinCache {
	return &SkinCache{
		cfg:      cfg,
		requests: make(chan *skinRequest, 256),
	}
}

func (c *SkinCache) Run(exitchan <-chan struct{}) {
	for {
		select {
		case <-exitchan:
			return
		case r := <-c.requests:
			r.ret(c.get(r.id, r.url))
		}
	}
}

func (c *SkinCache) getPath(id uuid.UUID) string {
	root := c.cfg.GetDString("SkinCache", "Root")
	return path.Join(root, uuidToString(id)+".png")
}

func getImageModTime(path string) (time.Time, error) {
	s, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return s.ModTime(), err
}

func getImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func saveImage(i image.Image, path string) error {
	buf := bytes.NewBufferString("")
	err := png.Encode(buf, i)
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func (c *SkinCache) fetchImage(url string) (image.Image, error) {
	cl := http.Client{
		Timeout: time.Second * 5,
	}
	log.Printf("Fetching: %q", url)
	resp, err := cl.Get(url)
	if err != nil {
		return nil, err
	}
	teximg, err := png.Decode(resp.Body)
	if err != nil {
		return nil, err
	}
	if c.cfg.GetDBool(false, "TrimToHead") {
		teximg, _ = CropImage(teximg, image.Rect(8, 8, 16, 16))
	}
	return teximg, nil
}

func (c *SkinCache) get(id uuid.UUID, url string) (image.Image, error) {
	p := c.getPath(id)
	mt, err := getImageModTime(p)
	if err != nil {
		return nil, err
	}
	if time.Since(mt) >= time.Hour*24*7 {
		i, err := c.fetchImage(url)
		if err != nil {
			return nil, err
		}
		return i, saveImage(i, p)
	} else {
		return getImage(p)
	}
}

func (c *SkinCache) GetSkinAsync(id uuid.UUID, url string, fn func(image.Image, error)) {
	c.requests <- &skinRequest{
		ret: fn,
		id:  id,
		url: url,
	}
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

func uuidToString(uuid [16]byte) string {
	var buf [36]byte
	encodeUUIDToHex(buf[:], uuid)
	return string(buf[:])
}

func encodeUUIDToHex(dst []byte, uuid [16]byte) {
	hex.Encode(dst[:], uuid[:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], uuid[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], uuid[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], uuid[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:], uuid[10:])
}
