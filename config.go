package main

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
)

type SourceConfiguration struct {
	Type    string
	Params  SourceParams
	Filters map[string]string
}

type HttpConfiguration struct {
	Port int
}

type CacheConfiguration struct {
	Directory string
}

type ImageConfiguration struct {
	Maxwidth  int
	Maxheight int
}

type Configuration struct {
	Sources        []SourceConfiguration
	Http           HttpConfiguration
	Directory      string
	Vars           map[string]string
	MetaTags       map[string]string `yaml:"meta-tags"`
	Cache          CacheConfiguration
	Image          ImageConfiguration
	UpdateInterval int `yaml:"update-interval"`
}

func (c Configuration) IndexTemplateName() string {
	return "index.html"
}

func (c Configuration) Validate() error {
	if len(c.Sources) < 1 {
		return errors.New("At least one source is required")
	}

	finfo, err := os.Stat(path.Join(c.TemplateDirectory(), c.IndexTemplateName()))
	if err == nil {
		if finfo.IsDir() {
			return errors.New(fmt.Sprintf("%v should be a file", c.IndexTemplateName()))
		}
	} else {
		return errors.New(fmt.Sprintf("%v template does not exist", c.IndexTemplateName()))
	}
	return nil
}

func (c Configuration) StaticFilesDirectory() string {
	return path.Join(c.Directory, "static")
}

func (c Configuration) TemplateDirectory() string {
	return path.Join(c.Directory, "templates")
}

func ReadConfiguration(directory string) (config Configuration, err error) {
	file, err := ioutil.ReadFile(path.Join(directory, "config.yml"))
	if err != nil {
		return
	}
	err = yaml.Unmarshal(file, &config)
	if err != nil {
		return
	}
	config.Directory = ExpandHome(directory)

	// set defaults when options are missing
	if config.Http.Port < 1 {
		config.Http.Port = 8007
	}
	config.Cache.Directory = ExpandHome(config.Cache.Directory)
	if config.Cache.Directory == "" {
		config.Cache.Directory = path.Join(config.Directory, "cache")
	}

	if config.Image.Maxwidth < 1 && config.Image.Maxheight < 1 {
		config.Image.Maxheight = 300
	}

	if config.UpdateInterval < 1 {
		// disabled per default
		config.UpdateInterval = 0
	}

	return
}
