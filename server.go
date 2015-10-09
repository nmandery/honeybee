package honeybee

import (
	"fmt"
	"github.com/julienschmidt/httprouter"
	"github.com/peterbourgon/diskv"
	"log"
	"net/http"
	"path"
	"text/template"
	"time"
)

type Server struct {
	config         *Configuration
	sources        Sources
	blockStore     BlockStore
	templ          *template.Template
	router         *httprouter.Router
	imgProxy       *ImgProxy
	doUpdatingChan chan bool
	cache          Cache
}

// create a new server from the configuration directory
func NewServer(config *Configuration) (srv *Server, err error) {
	err = config.Validate()
	if err != nil {
		log.Printf("configuration problem: %v\n", err)
		return
	}

	sources, err := CreateSources(config)
	if err != nil {
		log.Printf("Could setup sources: %v\n", err)
		return
	}

	templ, err := template.New("t").ParseGlob(path.Join(config.TemplateDirectory(), "*.html"))
	if err != nil {
		log.Printf("Could not setup templates: %v\n", err)
		return
	}

	err = EnsureDirectoryExists(config.Cache.Directory)
	if err != nil {
		log.Printf("Could not create cache directory: %v\n", err)
		return
	}
	cache := NewForgettingCache(
		diskv.New(diskv.Options{
			BasePath:     config.Cache.Directory,
			CacheSizeMax: 0,
			Transform:    cacheTransformKeyToPath,
		}), 10)

	imgProxy, err := NewImgProxy(config, cache)
	if err != nil {
		log.Printf("Could not setup caching proxy: %v\n", err)
		return
	}

	srv = &Server{
		config:         config,
		sources:        sources,
		blockStore:     NewBlockStore(),
		templ:          templ,
		router:         httprouter.New(),
		imgProxy:       imgProxy,
		doUpdatingChan: make(chan bool),
		cache:          cache,
	}

	// goroutine to update the blocks from the sources
	go func() {
		doUpdating := false
		updateTimeout := 10
		for {
			if doUpdating {
				log.Printf("Pulling sources.")

				err := srv.PullSources()
				if err != nil {
					log.Printf("Could not pull sources: %v", err)
				}
				if updateTimeout > 0 && srv.blockStore.Size() > 0 {
					updateTimeout = srv.config.UpdateInterval
				}
			}

			if updateTimeout > 0 {
				select {
				case doUpdating = <-srv.doUpdatingChan:
					continue
				case <-time.After(time.Second * time.Duration(updateTimeout)):
					continue
				}
			} else {
				doUpdating = <-srv.doUpdatingChan
			}
		}
	}()

	srv.router.GET("/", srv.handleIndexPage)
	srv.router.GET("/image/:id", srv.handleImageRequest)

	fileServer := http.FileServer(http.Dir(config.StaticFilesDirectory()))
	srv.router.Handler("GET", "/static/*filepath", http.StripPrefix("/static/", fileServer))
	// fallback to static files un-resolved requests in root directory - for files
	// like favicon.ico and robots.txt
	srv.router.NotFound = fileServer
	return
}

func (s *Server) StartUpdating() {
	s.doUpdatingChan <- true
}

func (s *Server) StopUpdating() {
	s.doUpdatingChan <- false
}

func (s *Server) PullSources() (err error) {
	s.cache.DeleteSome()

	// use the imageanalyser to fill the size attributes of the blocks
	// this also has the effect of pre-seeding the cache
	ia := NewImageAnalyzer(s.imgProxy)
	_ = s.sources.SendBlocksTo(ia)
	blocks, err := ia.GetBlocks()
	if err != nil {
		return
	}
	s.blockStore.ReceiveBlocks(blocks)
	return nil
}

// handle the request to an image
func (s *Server) handleImageRequest(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id := ps.ByName("id")
	if id == "" {
		log.Printf("imageRequest: Path variable id not found.\n")
		http.NotFound(w, r)
		return
	}
	block, found := s.blockStore.Get(id)
	if !found {
		http.NotFound(w, r)
		return
	}
	if !block.HasImage() {
		http.NotFound(w, r)
		return
	}
	//fmt.Fprintf(w, "id=%v, %v", id, found)

	err := s.imgProxy.ProxyImage(w, r, block.ImageLink)
	if err != nil {
		http.Error(w, "Could not read image from upstream server", http.StatusInternalServerError)
	}
}

// handle request to the index page
func (s *Server) handleIndexPage(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	indexPage := struct {
		Blocks   []*Block
		Vars     map[string]string
		MetaTags map[string]string
		Image    ImageConfiguration
	}{
		Blocks:   s.blockStore.List(),
		Vars:     s.config.Vars,
		MetaTags: s.config.MetaTags,
		Image:    s.config.Image,
	}
	s.templ.ExecuteTemplate(w, s.config.IndexTemplateName(), indexPage)
}

// implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// start the http server, listen on the port from the configuration
func (s *Server) Serve() error {
	bindTo := fmt.Sprintf(":%v", s.config.Http.Port)
	log.Printf("Listening on %v ...\n", bindTo)
	return http.ListenAndServe(bindTo, s)
}

// drop all contents in the cache
func (s *Server) DropCache() {
	s.cache.DeleteAll()
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
