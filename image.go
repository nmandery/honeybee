package honeybee

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/peterbourgon/diskv"
	"image"
	"log"
	"net/http"
	"os"
	"runtime"
	"willnorris.com/go/imageproxy"
)

type ImageProxyWrapper struct {
	proxy *imageproxy.Proxy
	// transform/resize/scale parameters for imageproxy requests
	proxyTransform string

	forgettingCache *ForgettingCache
}

// create a caching and resizing image proxy
func NewImageProxyWrapper(c *Configuration) (proxyWrapper *ImageProxyWrapper, err error) {
	err = ensureDirectoryExists(c.Cache.Directory)
	if err != nil {
		return
	}

	diskCache := diskv.New(diskv.Options{
		BasePath:     c.Cache.Directory,
		CacheSizeMax: 0,
		Transform:    cacheTransformKeyToPath,
	})
	cache := NewForgettingCache(diskCache, 10)
	proxy := imageproxy.NewProxy(nil, cache)

	proxyTransform := ""
	maxHeight := ""
	maxWidth := ""
	if c.Image.Maxwidth > 0 {
		maxWidth = fmt.Sprintf("%v", c.Image.Maxwidth)
	}
	if c.Image.Maxheight > 0 {
		maxHeight = fmt.Sprintf("%v", c.Image.Maxheight)
	}
	if maxHeight != "" || maxWidth != "" {
		proxyTransform = fmt.Sprintf("%vx%v", maxWidth, maxHeight)
	}
	log.Printf("Transforming images by \"%v\"\n", proxyTransform)

	proxyWrapper = &ImageProxyWrapper{
		proxy:           proxy,
		proxyTransform:  proxyTransform,
		forgettingCache: cache,
	}
	return
}

func (ipw *ImageProxyWrapper) ForgetSome() {
	ipw.forgettingCache.ForgetSome()
}

// load an external image or fetch it from the cache
// and write it to the ResponseWriter
func (ipw *ImageProxyWrapper) ProxyImage(w http.ResponseWriter, url string) (err error) {
	new_req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost/%v/%v", ipw.proxyTransform, url), nil)
	if err != nil {
		log.Printf("imageRequest: %v.\n", err)
		return
	}
	ipw.proxy.ServeHTTP(w, new_req)
	return nil
}

// return a image.Config instance of a cached image. If the image
// is not in the cache it will be fetched
func (ipw *ImageProxyWrapper) GetImageConfig(url string) (cfg image.Config, err error) {
	w := new(capturingResponseWriter)
	err = ipw.ProxyImage(w, url)
	if err != nil {
		return
	}
	cfg, _, err = image.DecodeConfig(&w.Body)
	return
}

// create a nested path for a cache key to avoid many
// inodes in one directory
func cacheTransformKeyToPath(s string) (parts []string) {
	partLen := 2
	depth := 3
	sLen := len(s)
	for i := 0; i < depth; i++ {
		if (partLen * (i + 1)) > sLen {
			parts = append(parts, "_")
			break
		}
		parts = append(parts, s[partLen*i:partLen*(i+1)])
	}
	return
}

// ensure a directory exists, create it if it does not
func ensureDirectoryExists(d string) (err error) {
	stat, err := os.Stat(d)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(d, 0744)
			if err != nil {
				return
			}
		} else {
			return
		}
	} else {
		if !stat.IsDir() {
			return errors.New(fmt.Sprintf("%v already exists, but is not a directory", d))
		}
	}
	return nil
}

type capturingResponseWriter struct {
	Body bytes.Buffer
}

func (crw *capturingResponseWriter) Header() http.Header {
	return http.Header{}
}

func (crw *capturingResponseWriter) Write(b []byte) (bw int, err error) {
	bw, err = crw.Body.Write(b)
	return
}

func (crw *capturingResponseWriter) WriteHeader(status int) {
	_ = status
}

type ImageAnalyzer struct {
	proxyWrapper *ImageProxyWrapper
	outBlocks    []*Block
}

func NewImageAnalyzer(proxyWrapper *ImageProxyWrapper) (ia *ImageAnalyzer) {
	return &ImageAnalyzer{
		proxyWrapper: proxyWrapper,
	}
}

func (ia *ImageAnalyzer) ReceiveBlocks(blocks []*Block) {
	in_chan := make(chan *Block)

	analyzeImageworker := func(in_chan chan *Block) {
		for block := range in_chan {
			block.ModifyMtx.Lock()
			defer block.ModifyMtx.Unlock()

			if block.HasImage() == false {
				continue
			}

			image_cfg, err := ia.proxyWrapper.GetImageConfig(block.ImageLink)
			if err != nil {
				log.Printf("Could not analzye image from %v. Cause: %v", block.ImageLink, err)
				continue
			}

			block.ImageWidth = image_cfg.Width
			block.ImageHeight = image_cfg.Height
		}
	}

	// start workers
	for wid := 0; wid < runtime.NumCPU()*2; wid++ {
		go analyzeImageworker(in_chan)
	}

	for _, block := range blocks {
		in_chan <- block
	}
	close(in_chan)

	ia.outBlocks = append(ia.outBlocks, blocks...)
}

func (ia *ImageAnalyzer) GetBlocks() ([]*Block, error) {
	return ia.outBlocks, nil
}
