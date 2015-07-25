package main

import (
	"fmt"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
	"path"
	"text/template"
	"time"
)

type Server struct {
	config         Configuration
	sources        Sources
	blockStore     BlockStore
	templ          *template.Template
	router         *httprouter.Router
	proxyWrapper   *ImageProxyWrapper
	doUpdatingChan chan bool
}

// create a new server from the configuration directory
func NewServer(configDirectory string) (srv *Server, err error) {
	config, err := ReadConfiguration(configDirectory)
	if err != nil {
		log.Printf("Could not read config file: %v\n", err)
		return
	}
	err = config.Validate()
	if err != nil {
		log.Printf("configuration problem: %v\n", err)
		return
	}

	sources, err := CreateSources(&config)
	if err != nil {
		log.Printf("Could setup sources: %v\n", err)
		return
	}

	templ, err := template.New("t").ParseGlob(path.Join(config.TemplateDirectory(), "*.html"))
	if err != nil {
		log.Printf("Could not setup templates: %v\n", err)
		return
	}

	proxyWrapper, err := NewImageProxyWrapper(&config)
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
		proxyWrapper:   proxyWrapper,
		doUpdatingChan: make(chan bool),
	}

	go func() {
		doUpdating := false
		updateTimeout := 10
		for {
			if doUpdating || srv.blockStore.Size() == 0 {
				log.Printf("Pulling sources.")

				err := srv.pullSources()
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

func (s *Server) pullSources() (err error) {
	// use the imageanalyser to fill the size attributes of the blocks
	// this also has the effect of pre-seeding the cache
	ia := NewImageAnalyzer(s.proxyWrapper)
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

	err := s.proxyWrapper.ProxyImage(w, block.ImageLink)
	if err != nil {
		http.Error(w, "could not create proxy request", http.StatusInternalServerError)
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
