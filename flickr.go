package honeybee

/*
Description of the "extra" parameters:
http://librdf.org/flickcurl/api/flickcurl-searching-search-extras.html

Note: The "o_dims" parameter is only returend when a "url_*" is also requested
and related directly to the size of that image.

How flickrs URL are assembled:
https://www.flickr.com/services/api/misc.urls.html
*/

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/azer/go-flickr"
	"strconv"
	"time"
)

const (
	FlickrUserPhotosSourceType   = "flickr-user-photos"
	FlickrUserPhotosetSourceType = "flickr-user-photoset"
	photosPerPage                = "200"
	photoExtras                  = "description,date_upload,o_dims,url_l,media,path_alias,original_format,owner_name"
)

type photoMessageContainer interface {
	PhotoList() []flickrPhoto
	Pages() int
	Owner() string
}

func checkSuccess(container photoMessageContainer, response *[]byte) (err error) {
	if len(container.PhotoList()) == 0 {
		// check if flickr api returned an error and try to unmarshal that
		var flickrErrorMsg flickrErrorMessage
		errerr := json.Unmarshal(*response, &flickrErrorMsg)
		if errerr == nil && flickrErrorMsg.Message != "" {
			err = errors.New(fmt.Sprintf("Flickr API: %v", flickrErrorMsg.Message))
		}
	}
	return
}

type flickrErrorMessage struct {
	Message string `json:"message"`
}

type flickrPhoto struct {
	Id              string `json:"id"`
	Title           string `json:"title,omitempty"`
	TimestampUpload string `json:"dateupload"`
	Description     struct {
		Content string `json:"_content,omitempty"`
	} `json:"description,omitempty"`
	Media    string `json:"media"`
	Owner    string `json:"owner"`
	ImageURL string `json:"url_l"`
	Height   string `json:"height_l"`
	Width    string `json:"width_l"`
}

type flickrPhotos struct {
	Pages int           `json:"pages"`
	Owner string        `json:"owner"`
	Photo []flickrPhoto `json:"photo"`
}

type flickrPeopleGetPublicPhotosMessage struct {
	Photos flickrPhotos `json:"photos"`
}

func (gppm flickrPeopleGetPublicPhotosMessage) PhotoList() []flickrPhoto {
	return gppm.Photos.Photo
}

func (gppm flickrPeopleGetPublicPhotosMessage) Pages() int {
	return gppm.Photos.Pages
}

func (gppm flickrPeopleGetPublicPhotosMessage) Owner() string {
	return ""
}

// response used for fetching photosets
type flickrPhotosetGetPhotosMessage struct {
	Photoset flickrPhotos `json:"photoset"`
}

func (gppm flickrPhotosetGetPhotosMessage) PhotoList() []flickrPhoto {
	return gppm.Photoset.Photo
}

func (gppm flickrPhotosetGetPhotosMessage) Pages() int {
	return gppm.Photoset.Pages
}

func (gppm flickrPhotosetGetPhotosMessage) Owner() string {
	return gppm.Photoset.Owner
}

type commonSourceParams struct {
	userName string
	key      string
	photoset string
}

func readCommonSourceParams(sourceType string, params *SourceParams) (*commonSourceParams, error) {
	userName := ""
	key := ""
	photoset := ""
	for k, v := range *params {
		switch k {
		case "key":
			key = v
		case "user":
			userName = v
		case "photoset":
			photoset = v
		default:
			err := errors.New(fmt.Sprintf("Unknown parameter for %v: %v", sourceType, k))
			return nil, err
		}
	}
	if userName == "" {
		err := errors.New("flickr source needs a user to fetch photos from")
		return nil, err
	}
	if key == "" {
		err := errors.New("flickr source needs a key")
		return nil, err
	}
	if (photoset == "") && (sourceType == FlickrUserPhotosetSourceType) {
		err := errors.New("flickr source needs a photoset to fetch photos from")
		return nil, err
	}
	csp := &commonSourceParams{
		userName: userName,
		key:      key,
		photoset: photoset,
	}
	return csp, nil
}

func pullBlocks(s Source, fetchPage func(int) (photoMessageContainer, error)) (blocks []*Block, err error) {
	page := 1
	for {
		flickrPhotos, err := fetchPage(page)
		if err != nil {
			return nil, err
		}
		for _, photo := range flickrPhotos.PhotoList() {

			// only photos are currently supported
			if photo.Media != "photo" {
				continue
			}

			// owner of photo is not always provided. f.e. not with photoset photos
			owner := photo.Owner
			if owner == "" {
				owner = flickrPhotos.Owner()
			}

			block := NewBlock(s)
			block.Title = photo.Title
			block.ImageLink = photo.ImageURL
			block.Link = fmt.Sprintf("https://www.flickr.com/photos/%v/%v",
				owner, photo.Id)
			block.Content = photo.Description.Content

			timestamp, err := strconv.ParseInt(photo.TimestampUpload, 0, 64)
			if err == nil {
				block.TimeStamp = time.Unix(timestamp, 0).UTC()
			}
			blocks = append(blocks, block)
		}

		// check if the last page has been reached
		if page >= flickrPhotos.Pages() {
			break
		}
		page++
	}
	return blocks, nil
}

type FlickrUserPhotosSource struct {
	userName string
	key      string
}

func (fs *FlickrUserPhotosSource) Type() string {
	return FlickrUserPhotosSourceType
}

func (fs *FlickrUserPhotosSource) Id() string {
	return IdEncodeStrings(fs.Type(), fs.userName, fs.key)
}

func NewFlickrUserPhotosSource(params SourceParams) (fs *FlickrUserPhotosSource, err error) {
	csp, err := readCommonSourceParams(FlickrUserPhotosSourceType, &params)
	if err != nil {
		return
	}
	fs = &FlickrUserPhotosSource{
		userName: csp.userName,
		key:      csp.key,
	}
	return fs, nil
}

func (fs *FlickrUserPhotosSource) GetBlocks() (blocks []*Block, err error) {
	client := flickr.Client{
		Key: fs.key,
	}
	fetchPage := func(page int) (container photoMessageContainer, err error) {
		response, err := client.Request("people.getPublicPhotos",
			flickr.Params{
				"user_id":  fs.userName,
				"per_page": photosPerPage,
				"page":     fmt.Sprintf("%v", page),
				"extras":   photoExtras,
			})
		if err != nil {
			return
		}
		//fmt.Printf("F: %v\n", string(response))
		var flickrPhotos flickrPeopleGetPublicPhotosMessage
		err = json.Unmarshal(response, &flickrPhotos)
		if err != nil {
			return
		}
		err = checkSuccess(flickrPhotos, &response)
		if err != nil {
			return
		}
		container = flickrPhotos
		return container, nil
	}
	blocks, err = pullBlocks(fs, fetchPage)
	return
}

type FlickrUserPhotosetSource struct {
	userName string
	key      string
	photoset string
}

func (fs *FlickrUserPhotosetSource) Type() string {
	return FlickrUserPhotosetSourceType
}

func (fs *FlickrUserPhotosetSource) Id() string {
	return IdEncodeStrings(fs.Type(), fs.userName, fs.key, fs.photoset)
}

func NewFlickrUserPhotosetSource(params SourceParams) (fs *FlickrUserPhotosetSource, err error) {
	csp, err := readCommonSourceParams(FlickrUserPhotosSourceType, &params)
	if err != nil {
		return
	}
	fs = &FlickrUserPhotosetSource{
		userName: csp.userName,
		key:      csp.key,
		photoset: csp.photoset,
	}
	return fs, nil
}

func (fs *FlickrUserPhotosetSource) GetBlocks() (blocks []*Block, err error) {
	client := flickr.Client{
		Key: fs.key,
	}
	fetchPage := func(page int) (container photoMessageContainer, err error) {
		response, err := client.Request("photosets.getPhotos",
			flickr.Params{
				"user_id":        fs.userName,
				"photoset_id":    fs.photoset,
				"privacy_filter": "1", // only public photos
				"media":          "photo",
				"per_page":       photosPerPage,
				"page":           fmt.Sprintf("%v", page),
				"extras":         photoExtras,
			})
		if err != nil {
			return
		}
		var photoset flickrPhotosetGetPhotosMessage
		err = json.Unmarshal(response, &photoset)
		if err != nil {
			return
		}
		err = checkSuccess(photoset, &response)
		if err != nil {
			return
		}
		container = photoset
		return container, nil
	}
	blocks, err = pullBlocks(fs, fetchPage)
	return
}
