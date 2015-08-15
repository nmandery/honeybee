package honeybee

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"time"
	"willnorris.com/go/imageproxy"
)

type download struct {
	httpResponseData []byte
	err              error
}

type downloadOperation struct {

	// channels of downstream listeners waiting for results
	downstreamChans []chan *download
	modifyMtx       *sync.Mutex
}

type ImgProxy struct {
	cache            Cache
	transformOptions *imageproxy.Options

	operations    map[string]*downloadOperation
	operationsMtx *sync.Mutex
}

// create a caching and resizing image proxy
func NewImgProxy(c *Configuration, cache Cache) (imgProxy *ImgProxy, err error) {
	imgProxy = &ImgProxy{
		cache: cache,
		transformOptions: &imageproxy.Options{
			Width:          float64(c.Image.Maxwidth),
			Height:         float64(c.Image.Maxheight),
			Fit:            false,
			Rotate:         0,
			FlipVertical:   false,
			FlipHorizontal: false,
			Quality:        c.Image.Quality,
			Signature:      "",
		},
		operations:    make(map[string]*downloadOperation),
		operationsMtx: new(sync.Mutex),
	}
	return
}

// id for a url to use in the cache
func (ipw *ImgProxy) cacheKey(url string) string {
	h := sha1.New()
	io.WriteString(h, url)
	io.WriteString(h, "|")
	io.WriteString(h, ipw.transformOptions.String())
	return hex.EncodeToString(h.Sum(nil))
}

// schedule an image to be fetched from upstream.
// this method returns a channel on which the download can be received.
// multiple request for the same url will be pooled, so an url
// is downloaded only once.
// The downloaded image will be transformed and cached.
func (ipw *ImgProxy) fetchFromUpstream(url string) chan *download {
	ipw.operationsMtx.Lock()
	defer ipw.operationsMtx.Unlock()

	dlOp, found := ipw.operations[url]
	if !found {
		dlOp = new(downloadOperation)
		dlOp.modifyMtx = new(sync.Mutex)
	}

	dlOp.modifyMtx.Lock()
	downstreamChan := make(chan *download)
	dlOp.downstreamChans = append(dlOp.downstreamChans, downstreamChan)
	dlOp.modifyMtx.Unlock()

	if !found {
		ipw.operations[url] = dlOp
		go ipw.downloadAndCache(url, dlOp)
	}
	return downstreamChan
}

func (ipw *ImgProxy) downloadAndCache(url string, dlOp *downloadOperation) {
	downloadedData := new(download)
	cacheKey := ipw.cacheKey(url)

	//log.Printf("Downloading %s (%s)", url, cacheKey)
	upstreamResp, err := http.Get(url)
	if err == nil {
		defer upstreamResp.Body.Close()

		imgData, err := ioutil.ReadAll(upstreamResp.Body)
		if err == nil {
			buf := new(bytes.Buffer)
			fmt.Fprintf(buf, "%s %s\n", upstreamResp.Proto, upstreamResp.Status)
			upstreamResp.Header.WriteSubset(buf, map[string]bool{"Content-Length": true})

			transformedImgData, err := imageproxy.Transform(imgData, *ipw.transformOptions)
			if err != nil {
				log.Printf("Unable to transform image from %s: %v", url, err)
				// return original response from server
				fmt.Fprintf(buf, "Content-Length: %d\n\n", len(imgData))
				buf.Write(imgData)
				ipw.cache.Delete(cacheKey)
			} else {
				// put transformed image in the cache and return transformed image
				fmt.Fprintf(buf, "Content-Length: %d\n\n", len(transformedImgData))
				buf.Write(transformedImgData)

				if upstreamResp.StatusCode < 400 {
					ipw.cache.Set(cacheKey, buf.Bytes())
				}
			}
			downloadedData.httpResponseData = buf.Bytes()
		} else {
			log.Printf("unable to read body of download from %s: %v", url, err)
			downloadedData.err = err
		}
	} else {
		log.Printf("unable to download %s: %v", url, err)
		downloadedData.err = err
	}

	// remove the download from the operations map
	ipw.operationsMtx.Lock()
	delete(ipw.operations, url)
	ipw.operationsMtx.Unlock()

	dlOp.modifyMtx.Lock()
	defer dlOp.modifyMtx.Unlock()
	for _, downstreamChan := range dlOp.downstreamChans {
		// send downloaded data to all waiting listeners on the channels
		downstreamChan <- downloadedData

	}
}

// load an external image or fetch it from the cache
// and write it to the ResponseWriter
func (ipw *ImgProxy) ProxyImage(w http.ResponseWriter, req *http.Request, url string) (err error) {
	cacheKey := ipw.cacheKey(url)
	xCacheHeader := "HIT"

	var resp *http.Response

	// attempt to read from cache
	cachedData, ok := ipw.cache.Get(cacheKey)
	if ok {
		b := bytes.NewBuffer(cachedData)
		resp, err = http.ReadResponse(bufio.NewReader(b), req)
		if err != nil {
			log.Printf("Unable to read cached entry for %s: %v (cacheKey: %s)", url, err, cacheKey)

			// remove any invalid data from the cache and
			// fetch it fresh from upstream
			ipw.cache.Delete(cacheKey)
			resp = nil
		}
	}

	// fetch from upstream
	if resp == nil {
		xCacheHeader = "MISS"

		downloadedData := <-ipw.fetchFromUpstream(url)
		if downloadedData.err != nil {
			return downloadedData.err
		}
		resp, err = http.ReadResponse(bufio.NewReader(bytes.NewBuffer(downloadedData.httpResponseData)), req)
		if err != nil {
			return err
		}
	}

	// write to responsewriter
	copyHeader(w, resp, "Last-Modified")
	copyHeader(w, resp, "Expires")
	copyHeader(w, resp, "Etag")
	w.Header()[http.CanonicalHeaderKey("X-Cache")] = []string{xCacheHeader}

	if is304 := check304(req, resp); is304 {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	copyHeader(w, resp, "Content-Length")
	copyHeader(w, resp, "Content-Type")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	return nil
}

// return a image.Config instance of a cached image. If the image
// is not in the cache it will be fetched
func (ipw *ImgProxy) GetImageConfig(url string) (cfg image.Config, err error) {
	var dummyReq *http.Request
	dummyReq, err = http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	recorder := httptest.NewRecorder()
	err = ipw.ProxyImage(recorder, dummyReq, url)
	if err != nil {
		return
	}
	cfg, _, err = image.DecodeConfig(bufio.NewReader(recorder.Body))
	return
}

func copyHeader(w http.ResponseWriter, r *http.Response, header string) {
	key := http.CanonicalHeaderKey(header)
	if value, ok := r.Header[key]; ok {
		w.Header()[key] = value
	}
}

// check304 checks whether we should send a 304 Not Modified in response to
// req, based on the response resp.  This is determined using the last modified
// time and the entity tag of resp.
func check304(req *http.Request, resp *http.Response) bool {
	// TODO(willnorris): if-none-match header can be a comma separated list
	// of multiple tags to be matched, or the special value "*" which
	// matches all etags
	etag := resp.Header.Get("Etag")
	if etag != "" && etag == req.Header.Get("If-None-Match") {
		return true
	}

	lastModified, err := time.Parse(time.RFC1123, resp.Header.Get("Last-Modified"))
	if err != nil {
		return false
	}
	ifModSince, err := time.Parse(time.RFC1123, req.Header.Get("If-Modified-Since"))
	if err != nil {
		return false
	}
	if lastModified.Before(ifModSince) {
		return true
	}

	return false
}

type ImageAnalyzer struct {
	imgProxy  *ImgProxy
	outBlocks []*Block
}

func NewImageAnalyzer(imgProxy *ImgProxy) (ia *ImageAnalyzer) {
	return &ImageAnalyzer{
		imgProxy: imgProxy,
	}
}

func (ia *ImageAnalyzer) ReceiveBlocks(blocks []*Block) { // TODO: rename to seed
	in_chan := make(chan *Block)

	analyzeImageworker := func(in_chan chan *Block) {
		for block := range in_chan {
			block.ModifyMtx.Lock()
			defer block.ModifyMtx.Unlock()

			if block.HasImage() == false {
				continue
			}

			image_cfg, err := ia.imgProxy.GetImageConfig(block.ImageLink)
			if err != nil {
				log.Printf("Could not analyze image from %v. Cause: %v", block.ImageLink, err)
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
