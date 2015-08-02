package honeybee

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
)

type SourceParams map[string]string

type Source interface {
	GetBlocks() ([]*Block, error)
	Type() string
	Id() string
}

type Sources []Source
type FilterFunc func(int, *Block) bool

func (sources *Sources) SendBlocksTo(receiver BlockReceiver) (err error) {
	sync_chan := make(chan error)

	pullSource := func(sourceIndex int) {
		blocks, pull_err := (*sources)[sourceIndex].GetBlocks()
		if pull_err != nil {
			log.Printf("Failed to fetch %v: %v\n", (*sources)[sourceIndex].Type(), pull_err)
		} else {
			receiver.ReceiveBlocks(blocks)
		}
		sync_chan <- pull_err
	}

	for idx := range *sources {
		go pullSource(idx)
	}

	// wait for all sources to finish
	for _ = range *sources {
		var source_err error
		source_err = <-sync_chan
		if err == nil {
			err = source_err
		}
	}
	return err
}

type FilteredSource struct {
	nestedSource Source
	filters      []FilterFunc
}

func (fs *FilteredSource) Type() string {
	return fs.nestedSource.Type()
}

func (fs *FilteredSource) Id() string {
	return fs.nestedSource.Id()
}

func (fs *FilteredSource) AddFilter(fn FilterFunc) {
	fs.filters = append(fs.filters, fn)
}

func (fs *FilteredSource) GetBlocks() (blocks []*Block, err error) {
	blocks, err = fs.nestedSource.GetBlocks()
	if err != nil {
		return
	}
	// sort, to have the list prepared for index-based filters like the
	// the "limit" filter
	sort.Sort(ByTimeStamp(blocks))

	for _, filter := range fs.filters {
		var newBlocks []*Block
		for idx, block := range blocks {
			if filter(idx, block) {
				newBlocks = append(newBlocks, block)
			}
		}
		blocks = newBlocks
	}
	return blocks, nil
}

// limit the number of blocks
func makeLimitFilter(filterParam string) (fn FilterFunc, err error) {
	limit, interr := strconv.ParseInt(filterParam, 10, 64)
	if interr != nil {
		err = errors.New(fmt.Sprintf("Could not parse limit value: %v\n", filterParam))
		return
	}
	fn = func(idx int, block *Block) bool {
		if idx >= int(limit) {
			return false
		}
		return true
	}
	return
}

// filter the title of blocks using a regular expression
func makeTitleFilter(filterParam string) (fn FilterFunc, err error) {
	re, reerr := regexp.Compile(filterParam)
	if reerr != nil {
		return nil, reerr
	}
	fn = func(idx int, block *Block) bool {
		return re.MatchString(block.Title)
	}
	return
}

// filter the content of blocks using a regular expression
func makeContentFilter(filterParam string) (fn FilterFunc, err error) {
	re, reerr := regexp.Compile(filterParam)
	if reerr != nil {
		return nil, reerr
	}
	fn = func(idx int, block *Block) bool {
		return re.MatchString(block.Content)
	}
	return
}

func CreateSources(config *Configuration) (sources Sources, err error) {
	for _, sourceconfig := range config.Sources {
		var source Source
		switch sourceconfig.Type {
		case GithubUserReposSourceType:
			source, err = NewGithubUserReposSource(sourceconfig.Params)
		case FlickrUserPhotosSourceType:
			source, err = NewFlickrUserPhotosSource(sourceconfig.Params)
		case FlickrUserPhotosetSourceType:
			source, err = NewFlickrUserPhotosetSource(sourceconfig.Params)
		default:
			err = errors.New(fmt.Sprintf("Unknown source type: %v\n", sourceconfig.Type))
			return
		}
		if err != nil {
			err = errors.New(fmt.Sprintf("Could not create %v source: %v\n", sourceconfig.Type, err))
			return
		}

		if len(sourceconfig.Filters) > 0 {
			filteredSource := &FilteredSource{
				nestedSource: source,
			}
			for filterName, filterParam := range sourceconfig.Filters {
				var fn FilterFunc
				switch filterName {
				case "limit":
					fn, err = makeLimitFilter(filterParam)
				case "title":
					fn, err = makeTitleFilter(filterParam)
				case "content":
					fn, err = makeContentFilter(filterParam)
				default:
					err = errors.New(fmt.Sprintf("Unknown filter: %v\n", filterName))
					return
				}
				if err != nil {
					return
				}
				filteredSource.AddFilter(fn)
			}
			source = filteredSource
		}
		sources = append(sources, source)
	}
	return
}
